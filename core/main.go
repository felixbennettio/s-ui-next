package core

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/felixbennettio/s-ui-next/logger"

	sb "github.com/sagernet/sing-box"
	"github.com/sagernet/sing-box/adapter"
	_ "github.com/sagernet/sing-box/experimental/clashapi"
	_ "github.com/sagernet/sing-box/experimental/v2rayapi"
	"github.com/sagernet/sing-box/log"
	"github.com/sagernet/sing-box/option"
	_ "github.com/sagernet/sing-box/transport/v2rayquic"
	"github.com/sagernet/sing/service"
)

var (
	globalCtx        context.Context
	inbound_manager  adapter.InboundManager
	outbound_manager adapter.OutboundManager
	service_manager  adapter.ServiceManager
	endpoint_manager adapter.EndpointManager
	router           adapter.Router
	factory          log.Factory
)

type Core struct {
	lifecycle sync.Mutex
	instance  atomic.Pointer[Box]
}

func NewCore() *Core {
	globalCtx = context.Background()
	globalCtx = sb.Context(globalCtx, InboundRegistry(), OutboundRegistry(), EndpointRegistry(), DNSTransportRegistry(), ServiceRegistry())
	return &Core{}
}

func (c *Core) GetCtx() context.Context {
	return globalCtx
}

func (c *Core) GetInstance() *Box {
	return c.instance.Load()
}

func (c *Core) Start(sbConfig []byte) error {
	c.lifecycle.Lock()
	defer c.lifecycle.Unlock()
	if c.IsRunning() {
		return nil
	}
	var opt option.Options
	err := opt.UnmarshalJSONContext(globalCtx, sbConfig)
	if err != nil {
		logger.Error("Unmarshal config err:", err.Error())
		return err
	}

	instance, err := NewBox(Options{
		Context: globalCtx,
		Options: opt,
	})
	if err != nil {
		return err
	}

	err = instance.Start()
	if err != nil {
		_ = instance.Close()
		return err
	}
	factory = instance.logFactory

	globalCtx = service.ContextWith(globalCtx, c)
	inbound_manager = service.FromContext[adapter.InboundManager](globalCtx)
	outbound_manager = service.FromContext[adapter.OutboundManager](globalCtx)
	service_manager = service.FromContext[adapter.ServiceManager](globalCtx)
	endpoint_manager = service.FromContext[adapter.EndpointManager](globalCtx)
	router = service.FromContext[adapter.Router](globalCtx)

	c.instance.Store(instance)
	return nil
}

// ValidateConfig parses and constructs the complete embedded sing-box
// configuration without opening listeners. It uses the exact registries and
// sing-box version used by the running panel.
func (c *Core) ValidateConfig(sbConfig []byte) error {
	ctx := context.Background()
	ctx = sb.Context(ctx, InboundRegistry(), OutboundRegistry(), EndpointRegistry(), DNSTransportRegistry(), ServiceRegistry())
	var opt option.Options
	if err := opt.UnmarshalJSONContext(ctx, sbConfig); err != nil {
		return err
	}
	instance, err := NewBox(Options{Context: ctx, Options: opt})
	if err != nil {
		return err
	}
	return instance.Close()
}

func (c *Core) Stop() error {
	c.lifecycle.Lock()
	defer c.lifecycle.Unlock()
	instance := c.instance.Swap(nil)
	if instance == nil {
		return nil
	}
	return instance.Close()
}

func (c *Core) IsRunning() bool {
	return c.instance.Load() != nil
}
