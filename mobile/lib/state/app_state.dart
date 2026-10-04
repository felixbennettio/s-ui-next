import 'dart:convert';
import 'dart:ui' as ui;

import 'package:flutter/foundation.dart';
import 'package:flutter_secure_storage/flutter_secure_storage.dart';

import '../core/api_client.dart';
import '../core/app_localizations.dart';
import '../core/connection_profile.dart';
import '../core/save_result.dart';

class AppState extends ChangeNotifier {
  static const _legacyProfileKey = 'sui.connection.profile.v1';
  static const _profilesKey = 'sui.connection.profiles.v2';
  static const _activeProfileKey = 'sui.connection.activeProfile.v2';
  static const _localeKey = 'sui.locale.v1';
  static const _storage = FlutterSecureStorage(aOptions: AndroidOptions());

  final profiles = <ConnectionProfile>[];
  ConnectionProfile? profile;
  ApiClient? api;
  Map<String, dynamic> bootstrap = {};
  String localeCode = _initialLocale();
  bool restoring = true;
  bool busy = false;
  String? error;
  int _profileIdSeed = 0;

  bool get connected => api != null && profile?.token.isNotEmpty == true;

  Future<void> restore() async {
    restoring = true;
    notifyListeners();
    try {
      final storedLocale = await _storage.read(key: _localeKey);
      if (storedLocale != null && storedLocale.isNotEmpty) {
        localeCode = storedLocale;
      }
      final restoredProfiles = await _readProfiles();
      profiles
        ..clear()
        ..addAll(restoredProfiles);
      if (profiles.isEmpty) {
        return;
      }

      final activeId = await _storage.read(key: _activeProfileKey);
      final preferred = _profileForId(activeId) ?? profiles.first;
      profile = preferred;

      Object? firstError;
      final candidates = <ConnectionProfile>[
        preferred,
        ...profiles.where((item) => item.id != preferred.id),
      ];
      for (final candidate in candidates) {
        if (candidate.token.isEmpty) continue;
        try {
          await _activateProfile(candidate, persist: true);
          error = null;
          firstError = null;
          break;
        } catch (exception) {
          firstError ??= exception;
        }
      }
      if (api == null && firstError != null) {
        error = firstError.toString();
      }
      await _persistProfiles(activeId: api == null ? null : profile?.id);
    } catch (exception) {
      error = exception.toString();
      api = null;
      bootstrap = {};
    } finally {
      restoring = false;
      notifyListeners();
    }
  }

  Future<void> setLocale(String code) async {
    localeCode = code;
    await _storage.write(key: _localeKey, value: code);
    notifyListeners();
  }

  Future<void> connectWithToken(ConnectionProfile next) async {
    await _connect(next);
  }

  Future<void> switchProfile(ConnectionProfile next) async {
    if (connected && _sameProfile(next, profile)) return;
    await _connect(next);
  }

  void prepareNewConnection() {
    profile = null;
    api = null;
    bootstrap = {};
    error = null;
    notifyListeners();
  }

  Future<bool> connectWithCredentials(
    ConnectionProfile next,
    String username,
    String password,
    {String code = ''}
  ) async {
    busy = true;
    error = null;
    notifyListeners();
    try {
      _validateBaseUrl(next);
      final login = await ApiClient.login(
        profile: next,
        username: username,
        password: password,
        code: code,
        localeCode: localeCode,
      );
      if (login['requires2FA'] == true) {
        return true;
      }
      final token = login['token']?.toString() ?? '';
      if (token.isEmpty) throw ApiException(AppLocalizations.tr(localeCode, 'error.noToken'));
      await _connect(next.copyWith(token: token), manageBusy: false);
      return false;
    } catch (exception) {
      error = exception.toString();
      rethrow;
    } finally {
      busy = false;
      notifyListeners();
    }
  }

  Future<void> _connect(ConnectionProfile next, {bool manageBusy = true}) async {
    if (manageBusy) {
      busy = true;
      error = null;
      notifyListeners();
    }
    try {
      _validateBaseUrl(next);
      if (next.token.trim().isEmpty) throw ApiException(AppLocalizations.tr(localeCode, 'error.tokenRequired'));
      await _activateProfile(_prepareProfile(next), persist: true);
    } catch (exception) {
      error = exception.toString();
      rethrow;
    } finally {
      if (manageBusy) {
        busy = false;
        notifyListeners();
      }
    }
  }

  void _validateBaseUrl(ConnectionProfile next) {
    if (next.normalizedBaseUrl.isEmpty) {
      throw ApiException(AppLocalizations.tr(localeCode, 'error.urlRequired'));
    }
    if (!next.hasValidBaseUrl) {
      throw ApiException(AppLocalizations.tr(localeCode, 'error.urlInvalid'));
    }
  }

  Future<void> _activateProfile(ConnectionProfile next, {required bool persist}) async {
    final normalized = next.copyWith(baseUrl: next.normalizedBaseUrl);
    final client = ApiClient(normalized, localeCode: localeCode);
    await client.get('meta');

    final previousProfile = profile;
    final previousApi = api;
    final previousBootstrap = Map<String, dynamic>.from(bootstrap);
    profile = normalized;
    api = client;
    try {
      await refreshBootstrap(notify: false);
    } catch (_) {
      profile = previousProfile;
      api = previousApi;
      bootstrap = previousBootstrap;
      rethrow;
    }

    final index = profiles.indexWhere((item) => _sameProfile(item, normalized));
    if (index >= 0) {
      profiles[index] = normalized;
    } else {
      profiles.add(normalized);
    }
    if (persist) {
      await _persistProfiles(activeId: normalized.id);
    }
  }

  Future<void> refreshBootstrap({bool notify = true}) async {
    final client = api;
    if (client == null) return;
    if (notify) {
      busy = true;
      notifyListeners();
    }
    try {
      final next = Map<String, dynamic>.from(await client.get('bootstrap') as Map);
      if (api != client) return;
      bootstrap = next;
      error = null;
    } catch (exception) {
      if (api == client) error = exception.toString();
      rethrow;
    } finally {
      if (notify) {
        busy = false;
        notifyListeners();
      }
    }
  }

  Future<dynamic> getResource(String resource, {String? id}) {
    return api!.get('resources/$resource', query: {if (id != null) 'id': id});
  }

  Future<SaveResult> saveResource(
    String resource,
    String action,
    dynamic data, {
    List<int> initUsers = const [],
    bool apply = true,
  }) async {
    final client = api!;
    final value = await client.post('resources/$resource', data: {
      'action': action,
      'data': data,
      if (initUsers.isNotEmpty) 'initUsers': initUsers,
      'apply': apply,
    });
    var refreshFailed = value is Map && value['warning'] == 'savedButRefreshFailed';
    if (api == client) {
      // Keep the committed resource changes even if the subsequent refresh fails.
      if (value is Map && value['resources'] is Map) {
        bootstrap['panel'] = <String, dynamic>{
          if (bootstrap['panel'] is Map) ...Map<String, dynamic>.from(bootstrap['panel'] as Map),
          ...Map<String, dynamic>.from(value['resources'] as Map),
        };
      }
      try {
        await refreshBootstrap(notify: false);
      } catch (_) {
        refreshFailed = true;
      }
      if (api == client) notifyListeners();
    }
    return SaveResult(refreshFailed: refreshFailed, applied: apply && resource != 'settings');
  }

  Future<void> disconnect({bool revoke = false}) async {
    final current = profile;
    if (revoke && api != null) {
      try {
        await api!.delete('auth/token');
      } catch (_) {
        // Local logout must still succeed if the panel is offline.
      }
    }
    if (revoke && current != null) {
      profiles.removeWhere((item) => _sameProfile(item, current));
    }
    profile = null;
    api = null;
    bootstrap = {};
    error = null;
    await _persistProfiles(activeId: null);
    notifyListeners();
  }

  Future<void> removeProfile(ConnectionProfile target) async {
    final removingActive = _sameProfile(target, profile);
    profiles.removeWhere((item) => _sameProfile(item, target));
    if (removingActive) {
      profile = null;
      api = null;
      bootstrap = {};
      error = null;
    }
    await _persistProfiles(activeId: removingActive ? null : profile?.id);
    notifyListeners();
  }

  void reconfigure() {
    api = null;
    bootstrap = {};
    error = null;
    notifyListeners();
  }

  Future<List<ConnectionProfile>> _readProfiles() async {
    final rawProfiles = await _storage.read(key: _profilesKey);
    if (rawProfiles != null && rawProfiles.isNotEmpty) {
      final decoded = jsonDecode(rawProfiles);
      if (decoded is List) {
        return _normalizeProfiles(
          decoded
              .whereType<Map>()
              .map((item) => ConnectionProfile.fromJson(Map<String, dynamic>.from(item))),
        );
      }
    }

    final legacy = await _storage.read(key: _legacyProfileKey);
    if (legacy != null && legacy.isNotEmpty) {
      return _normalizeProfiles([ConnectionProfile.decode(legacy)]);
    }
    return const [];
  }

  Future<void> _persistProfiles({required String? activeId}) async {
    final normalized = _normalizeProfiles(profiles);
    profiles
      ..clear()
      ..addAll(normalized);
    await _storage.delete(key: _legacyProfileKey);
    if (profiles.isEmpty) {
      await _storage.delete(key: _profilesKey);
      await _storage.delete(key: _activeProfileKey);
      return;
    }
    await _storage.write(
      key: _profilesKey,
      value: jsonEncode(profiles.map((item) => item.toJson()).toList()),
    );
    if (activeId == null || activeId.isEmpty) {
      await _storage.delete(key: _activeProfileKey);
    } else {
      await _storage.write(key: _activeProfileKey, value: activeId);
    }
  }

  ConnectionProfile _prepareProfile(ConnectionProfile next) {
    final existing = _matchingProfile(next);
    final id = next.id.trim().isNotEmpty ? next.id.trim() : existing?.id ?? _newProfileId();
    return next.copyWith(id: id, baseUrl: next.normalizedBaseUrl);
  }

  ConnectionProfile? _profileForId(String? id) {
    if (id == null || id.isEmpty) return null;
    for (final item in profiles) {
      if (item.id == id) return item;
    }
    return null;
  }

  ConnectionProfile? _matchingProfile(ConnectionProfile candidate) {
    if (candidate.id.trim().isNotEmpty) {
      final byId = _profileForId(candidate.id.trim());
      if (byId != null) return byId;
    }
    for (final item in profiles) {
      if (item.normalizedBaseUrl == candidate.normalizedBaseUrl && item.name == candidate.name) {
        return item;
      }
    }
    return null;
  }

  bool _sameProfile(ConnectionProfile? first, ConnectionProfile? second) {
    if (first == null || second == null) return false;
    if (first.id.isNotEmpty && second.id.isNotEmpty) return first.id == second.id;
    return first.normalizedBaseUrl == second.normalizedBaseUrl && first.name == second.name;
  }

  List<ConnectionProfile> _normalizeProfiles(Iterable<ConnectionProfile> items) {
    final result = <ConnectionProfile>[];
    final usedIds = <String>{};
    final seenKeys = <String, int>{};
    for (final item in items) {
      final baseUrl = item.normalizedBaseUrl;
      if (baseUrl.isEmpty) continue;
      var id = item.id.trim();
      if (id.isEmpty || usedIds.contains(id)) {
        id = _newProfileId();
      }
      final profileKey = '$baseUrl\n${item.name}';
      final normalized = item.copyWith(id: id, baseUrl: baseUrl);
      final existingIndex = seenKeys[profileKey];
      if (existingIndex == null) {
        seenKeys[profileKey] = result.length;
        usedIds.add(id);
        result.add(normalized);
      } else {
        result[existingIndex] = normalized.copyWith(id: result[existingIndex].id);
      }
    }
    return result;
  }

  String _newProfileId() {
    _profileIdSeed += 1;
    return 'panel-${DateTime.now().microsecondsSinceEpoch.toRadixString(36)}-$_profileIdSeed';
  }
}

String _initialLocale() {
  final locale = ui.PlatformDispatcher.instance.locale;
  if (locale.languageCode == 'zh') return locale.scriptCode == 'Hant' ? 'zhHant' : 'zhHans';
  const supported = {'en', 'ja', 'fr', 'la', 'fa', 'vi', 'ru'};
  return supported.contains(locale.languageCode) ? locale.languageCode : 'en';
}
