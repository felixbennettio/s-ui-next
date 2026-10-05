package service

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/felixbennettio/s-ui-next/database"
	"github.com/felixbennettio/s-ui-next/database/model"
	"github.com/felixbennettio/s-ui-next/logger"
	"github.com/felixbennettio/s-ui-next/util/common"
	"github.com/pquerna/otp/totp"
	"golang.org/x/crypto/bcrypt"
)

type UserService struct{}

const (
	MaxTokenExpiryDays       int64 = 3650
	MaxTokenDescriptionRunes       = 128
)

type pendingTOTPEntry struct {
	Secret string
	Expiry time.Time
}

var (
	pendingTOTP       sync.Map
	recoveryCodeMu    sync.Mutex
	dummyPasswordHash = func() string {
		value, err := bcrypt.GenerateFromPassword([]byte("s-ui-next-invalid-password"), bcrypt.DefaultCost)
		if err != nil {
			panic("unable to initialize password verifier: " + err.Error())
		}
		return string(value)
	}()
)

func hashPassword(password string) (string, error) {
	value, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(value), err
}

func verifyPassword(stored, password string) bool {
	if strings.HasPrefix(stored, "$2a$") || strings.HasPrefix(stored, "$2b$") || strings.HasPrefix(stored, "$2y$") {
		return bcrypt.CompareHashAndPassword([]byte(stored), []byte(password)) == nil
	}
	matched := subtle.ConstantTimeCompare([]byte(stored), []byte(password)) == 1
	_ = bcrypt.CompareHashAndPassword([]byte(dummyPasswordHash), []byte(password))
	return matched
}

func (s *UserService) GetFirstUser() (*model.User, error) {
	db := database.GetDB()

	user := &model.User{}
	err := db.Model(model.User{}).
		First(user).
		Error
	if err != nil {
		return nil, err
	}
	return user, nil
}

func (s *UserService) UpdateFirstUser(username string, password string) error {
	if username == "" {
		return common.NewError("username can not be empty")
	} else if password == "" {
		return common.NewError("password can not be empty")
	}
	db := database.GetDB()
	user := &model.User{}
	err := db.Model(model.User{}).First(user).Error
	if database.IsNotFound(err) {
		hashed, hashErr := hashPassword(password)
		if hashErr != nil {
			return hashErr
		}
		user.Username = username
		user.Password = hashed
		return db.Model(model.User{}).Create(user).Error
	} else if err != nil {
		return err
	}
	hashed, err := hashPassword(password)
	if err != nil {
		return err
	}
	user.Username = username
	user.Password = hashed
	return db.Save(user).Error
}

func (s *UserService) Login(username string, password string, remoteIP string) (string, error) {
	user, err := s.CheckPassword(username, password, remoteIP)
	if err != nil {
		return "", err
	}
	s.RecordLogin(user.Username, remoteIP)
	return user.Username, nil
}

func (s *UserService) CheckUser(username string, password string, remoteIP string) *model.User {
	user, _ := s.CheckPassword(username, password, remoteIP)
	return user
}

func (s *UserService) CheckPassword(username string, password string, remoteIP string) (*model.User, error) {
	db := database.GetDB()

	user := &model.User{}
	err := db.Model(model.User{}).
		Where("username = ?", username).
		First(user).
		Error
	if database.IsNotFound(err) {
		_ = verifyPassword(dummyPasswordHash, password)
		return nil, common.NewError("wrong user or password! IP: ", remoteIP)
	} else if err != nil {
		logger.Warning("check user err:", err, " IP: ", remoteIP)
		return nil, err
	}
	if !verifyPassword(user.Password, password) {
		return nil, common.NewError("wrong user or password! IP: ", remoteIP)
	}
	if !strings.HasPrefix(user.Password, "$2") {
		if hashed, hashErr := hashPassword(password); hashErr == nil {
			db.Model(model.User{}).Where("id = ?", user.Id).Update("password", hashed)
			user.Password = hashed
		}
	}

	return user, nil
}

func (s *UserService) RecordLogin(username, remoteIP string) {
	lastLoginTxt := time.Now().Format("2006-01-02 15:04:05") + " " + remoteIP
	err := database.GetDB().Model(model.User{}).
		Where("username = ?", username).
		Update("last_logins", &lastLoginTxt).Error
	if err != nil {
		logger.Warning("unable to log login data", err)
	}
}

func (s *UserService) GetUsers() (*[]model.User, error) {
	var users []model.User
	db := database.GetDB()
	err := db.Model(model.User{}).Select("id,username,last_logins,totp_enabled").Scan(&users).Error
	if err != nil {
		return nil, err
	}
	return &users, nil
}

func (s *UserService) ChangePass(id string, oldPass string, newUser string, newPass string) error {
	db := database.GetDB()
	user := &model.User{}
	err := db.Model(model.User{}).Where("id = ?", id).First(user).Error
	if err != nil || database.IsNotFound(err) {
		return err
	}
	if !verifyPassword(user.Password, oldPass) {
		return common.NewError("wrong current password")
	}
	hashed, err := hashPassword(newPass)
	if err != nil {
		return err
	}
	user.Username = newUser
	user.Password = hashed
	return db.Save(user).Error
}

func (s *UserService) BeginTOTP(username, issuer string) (map[string]string, error) {
	if issuer == "" {
		issuer = "S-UI Next"
	}
	key, err := totp.Generate(totp.GenerateOpts{Issuer: issuer, AccountName: username})
	if err != nil {
		return nil, err
	}
	pendingTOTP.Store(username, pendingTOTPEntry{Secret: key.Secret(), Expiry: time.Now().Add(10 * time.Minute)})
	return map[string]string{"secret": key.Secret(), "uri": key.URL()}, nil
}

func recoveryHash(code string) string {
	sum := sha256.Sum256([]byte(strings.ToUpper(strings.TrimSpace(code))))
	return hex.EncodeToString(sum[:])
}

func (s *UserService) EnableTOTP(username, code string) ([]string, error) {
	raw, ok := pendingTOTP.Load(username)
	if !ok {
		return nil, common.NewError("TOTP enrollment expired")
	}
	pending := raw.(pendingTOTPEntry)
	if time.Now().After(pending.Expiry) || !totp.Validate(strings.TrimSpace(code), pending.Secret) {
		return nil, common.NewError("invalid TOTP code")
	}
	codes := make([]string, 10)
	hashes := make([]string, 10)
	for i := range codes {
		codes[i] = strings.ToUpper(common.Random(10))
		hashes[i] = recoveryHash(codes[i])
	}
	encoded, _ := json.Marshal(hashes)
	err := database.GetDB().Model(&model.User{}).Where("username = ?", username).Updates(map[string]interface{}{
		"totp_secret": pending.Secret, "totp_enabled": true, "recovery_codes": encoded,
	}).Error
	if err == nil {
		pendingTOTP.Delete(username)
	}
	return codes, err
}

func (s *UserService) VerifySecondFactor(user *model.User, code string) bool {
	code = strings.TrimSpace(code)
	if !user.TOTPEnabled {
		return true
	}
	if totp.Validate(code, user.TOTPSecret) {
		return true
	}
	recoveryCodeMu.Lock()
	defer recoveryCodeMu.Unlock()
	if err := database.GetDB().Where("id = ?", user.Id).First(user).Error; err != nil {
		return false
	}
	var hashes []string
	if json.Unmarshal(user.RecoveryCodes, &hashes) != nil {
		return false
	}
	wanted := recoveryHash(code)
	for index, hash := range hashes {
		if hash == wanted {
			hashes = append(hashes[:index], hashes[index+1:]...)
			encoded, _ := json.Marshal(hashes)
			return database.GetDB().Model(&model.User{}).Where("id = ?", user.Id).Update("recovery_codes", encoded).Error == nil
		}
	}
	return false
}

func (s *UserService) DisableTOTP(username, password, code, remoteIP string) error {
	user, err := s.CheckPassword(username, password, remoteIP)
	if err != nil {
		return err
	}
	if !s.VerifySecondFactor(user, code) {
		return common.NewError("invalid TOTP or recovery code")
	}
	return database.GetDB().Model(&model.User{}).Where("id = ?", user.Id).Updates(map[string]interface{}{
		"totp_secret": "", "totp_enabled": false, "recovery_codes": json.RawMessage("[]"),
	}).Error
}

func (s *UserService) LoadTokens() ([]byte, error) {
	db := database.GetDB()
	var tokens []model.Tokens
	err := db.Model(model.Tokens{}).Preload("User").Where("expiry == 0 or expiry > ?", time.Now().Unix()).Find(&tokens).Error
	if err != nil {
		return nil, err
	}
	var result []map[string]interface{}
	for _, t := range tokens {
		result = append(result, map[string]interface{}{
			"token":    t.Token,
			"expiry":   t.Expiry,
			"username": t.User.Username,
		})
	}
	jsonResult, _ := json.MarshalIndent(result, "", "  ")
	return jsonResult, nil
}

func (s *UserService) GetUserTokens(username string) (*[]model.Tokens, error) {
	db := database.GetDB()
	var token []model.Tokens
	err := db.Model(model.Tokens{}).Select("id,desc,'****' as token,expiry,user_id").Where("user_id = (select id from users where username = ?)", username).Find(&token).Error
	if err != nil && !database.IsNotFound(err) {
		println(err.Error())
		return nil, err
	}
	return &token, nil
}

func (s *UserService) AddToken(username string, expiry int64, desc string) (string, error) {
	username = strings.TrimSpace(username)
	desc = strings.TrimSpace(desc)
	if username == "" {
		return "", common.NewError("username can not be empty")
	}
	if err := ValidateTokenOptions(expiry, desc); err != nil {
		return "", err
	}
	db := database.GetDB()
	var user model.User
	if err := db.Model(model.User{}).Select("id").Where("username = ?", username).First(&user).Error; err != nil {
		return "", err
	}
	if expiry > 0 {
		expiry = expiry*86400 + time.Now().Unix()
	}
	token := &model.Tokens{
		Token:  common.Random(32),
		Desc:   desc,
		Expiry: expiry,
		UserId: user.Id,
	}
	err := db.Create(token).Error
	if err != nil {
		return "", err
	}
	return token.Token, nil
}

func ValidateTokenOptions(expiryDays int64, description string) error {
	if expiryDays < 0 || expiryDays > MaxTokenExpiryDays {
		return common.NewErrorf("expiryDays must be between 0 and %d", MaxTokenExpiryDays)
	}
	if len([]rune(strings.TrimSpace(description))) > MaxTokenDescriptionRunes {
		return common.NewErrorf("token description must be at most %d characters", MaxTokenDescriptionRunes)
	}
	return nil
}

func (s *UserService) DeleteToken(id string) error {
	db := database.GetDB()
	return db.Model(model.Tokens{}).Where("id = ?", id).Delete(&model.Tokens{}).Error
}

// DeleteUserToken prevents one administrator token from deleting another
// administrator's token by guessing its numeric id.
func (s *UserService) DeleteUserToken(username string, id string) error {
	db := database.GetDB()
	return db.Model(model.Tokens{}).
		Where("id = ? AND user_id = (SELECT id FROM users WHERE username = ?)", id, username).
		Delete(&model.Tokens{}).Error
}

func (s *UserService) DeleteTokenValue(username string, token string) error {
	db := database.GetDB()
	return db.Model(model.Tokens{}).
		Where("token = ? AND user_id = (SELECT id FROM users WHERE username = ?)", token, username).
		Delete(&model.Tokens{}).Error
}
