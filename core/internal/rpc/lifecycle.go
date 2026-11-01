package rpc

import (
	"context"
	"errors"
	"fmt"
	"log"
	"runtime"
	"time"

	"PorosCore/gen"
	"PorosCore/internal/boxmain"
	"PorosCore/internal/xray"

	"github.com/sagernet/sing-box/option"
)

func (s *server) Start(ctx context.Context, in *gen.LoadConfigReq) (out *gen.ErrorResp, _ error) {
	lifecycleMu.Lock()
	defer lifecycleMu.Unlock()

	var err error

	defer func() {
		out = &gen.ErrorResp{}
		if err != nil {
			out.Error = To(err.Error())
			setBoxInstance(nil, nil, nil)
		}
	}()

	if debugLog {
		log.Println("Start:", *in.CoreConfig)
		if in.XrayConfig != nil {
			log.Println("Start Xray:", *in.XrayConfig)
		}
	}

	if currentBox() != nil {
		err = errors.New("instance already started")
		return
	}

	// Filled in below, once boxmain.Create has built the box these sidecars resolve through.
	var boxCtx boxContextHolder
	prepareXray := xrayPreparer(in.GetXrayOutboundDnsStrategy(), boxCtx.get)

	if *in.NeedXray {
		if in.GetXrayLazyStart() {
			gate, e := xray.StartGate(*in.XrayConfig,
				time.Duration(in.GetXrayIdleSeconds())*time.Second, prepareXray)
			if e != nil {
				err = e
				return
			}
			setXray(nil, gate)
		} else {
			instance, e := xray.CreateXrayInstance(*in.XrayConfig)
			if e != nil {
				err = e
				return
			}
			if e = prepareXray(instance); e != nil {
				instance.Close()
				err = e
				return
			}
			if e = instance.Start(); e != nil {
				instance.Close()
				err = e
				return
			}
			setXray(instance, nil)
		}
	}

	if fullConfigs := in.GetXrayFullConfigs(); len(fullConfigs) > 0 {
		gates, e := startXrayFullGates(fullConfigs,
			time.Duration(in.GetXrayFullIdleSeconds())*time.Second, prepareXray)
		if e != nil {
			closeXray()
			err = e
			return
		}
		setXrayFullGates(gates)
	}

	var routeRules []option.Rule
	box, cancel, err := boxmain.Create([]byte(*in.CoreConfig), boxCtx.publish, func(options *option.Options) {
		if options.Route != nil {
			routeRules = options.Route.Rules
		}
	})
	if err != nil {
		closeXray()
		return
	}
	setBoxInstance(box, cancel, routeRules)

	if in.GetTunIpv4Cidr() != "" {
		stopEgressForwardingWatch = watchEgressForwarding(box.Network())
	}

	return
}

func (s *server) Stop(ctx context.Context, in *gen.EmptyReq) (out *gen.ErrorResp, _ error) {
	lifecycleMu.Lock()
	defer lifecycleMu.Unlock()

	var err error

	defer func() {
		out = &gen.ErrorResp{}
		if err != nil {
			out.Error = To(err.Error())
		}
	}()

	box, cancel := currentInstance()
	if box == nil {
		return
	}

	if stopEgressForwardingWatch != nil {
		stopEgressForwardingWatch()
		stopEgressForwardingWatch = nil
	}

	// Unpublished first, so a poll mid-teardown sees no instance rather than a dying one.
	setBoxInstance(nil, nil, nil)
	box.CloseWithTimeout(cancel, time.Second*2, log.Println, true)

	closeXray()

	return
}

func (s *server) CheckConfig(ctx context.Context, in *gen.LoadConfigReq) (out *gen.ErrorResp, _ error) {
	out = &gen.ErrorResp{}
	// boxmain.Check can panic on malformed configs; unrecovered it reaches main()'s os.Exit(0) and kills the core.
	defer func() {
		if r := recover(); r != nil {
			buf := make([]byte, 4096)
			n := runtime.Stack(buf, false)
			log.Printf("CheckConfig panic: %v\n%s", r, buf[:n])
			out.Error = To(fmt.Sprintf("CheckConfig panic: %v", r))
		}
	}()
	if in.GetNeedXray() {
		if err := xray.CheckXrayConfig(in.GetXrayConfig()); err != nil {
			out.Error = To(err.Error())
		}
		return
	}
	err := boxmain.Check([]byte(*in.CoreConfig))
	if err != nil {
		out.Error = To(err.Error())
	}
	return
}
