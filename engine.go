package goddgs

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type EngineOptions struct {
	Providers []Provider
	Hooks     []EventHook
}

type Engine struct {
	providers []Provider
	hooks     []EventHook
}

func NewEngine(opts EngineOptions) (*Engine, error) {
	if len(opts.Providers) == 0 {
		return nil, fmt.Errorf("goddgs: at least one provider is required")
	}
	providers := make([]Provider, 0, len(opts.Providers))
	for _, p := range opts.Providers {
		if p != nil {
			providers = append(providers, p)
		}
	}
	if len(providers) == 0 {
		return nil, fmt.Errorf("goddgs: at least one non-nil provider is required")
	}
	return &Engine{providers: providers, hooks: opts.Hooks}, nil
}

func (e *Engine) Search(ctx context.Context, req SearchRequest) (SearchResponse, error) {
	if strings.TrimSpace(req.Query) == "" {
		return SearchResponse{}, &SearchError{Kind: ErrKindInvalidInput, Provider: "engine", Temporary: false, Cause: fmt.Errorf("query is required")}
	}
	if req.MaxResults <= 0 {
		req.MaxResults = 10
	}
	if req.Region == "" {
		req.Region = "us-en"
	}
	started := time.Now()
	e.emit(Event{Type: EventSearchStart})
	defer func() {
		e.emit(Event{Type: EventSearchFinish, Duration: time.Since(started)})
	}()

	diag := Diagnostics{Timings: map[string]time.Duration{}, ProviderChain: make([]string, 0, len(e.providers))}
	for idx, p := range e.providers {
		if p == nil || !p.Enabled() {
			continue
		}
		name := p.Name()
		diag.ProviderChain = append(diag.ProviderChain, name)
		e.emit(Event{Type: EventProviderStart, Provider: name})
		ps := time.Now()
		results, err := p.Search(ctx, req)
		dur := time.Since(ps)
		diag.Timings[name] = dur
		diag.Attempts++
		if err == nil && len(results) > 0 {
			e.emit(Event{Type: EventProviderEnd, Provider: name, Duration: dur, Success: true})
			return SearchResponse{Results: results, Provider: name, FallbackUsed: idx > 0, Diagnostics: diag}, nil
		}
		se := classifyError(name, err)
		if se == nil {
			se = &SearchError{Kind: ErrKindNoResults, Provider: name, Cause: ErrNoResults}
		}
		diag.Errors = append(diag.Errors, ProviderError{
			Provider: name,
			Kind:     se.Kind,
			Message:  se.Error(),
			Details:  se.Details,
		})
		var binfo *BlockInfo
		if se.Kind == ErrKindBlocked {
			bi := BlockInfo{Signal: BlockSignalGeneric, DetectorKey: "blocked"}
			if se.Details != nil && se.Details["detector"] != "" {
				bi.DetectorKey = se.Details["detector"]
			}
			binfo = &bi
			diag.BlockInfo = binfo
			e.emit(Event{Type: EventBlocked, Provider: name, ErrKind: se.Kind, Block: binfo})
		}
		e.emit(Event{Type: EventProviderEnd, Provider: name, Duration: dur, Success: false, ErrKind: se.Kind, Block: binfo})
		if idx < len(e.providers)-1 {
			e.emit(Event{Type: EventFallback, Provider: name, ErrKind: se.Kind})
		}
	}
	if len(diag.Errors) == 0 {
		return SearchResponse{Diagnostics: diag}, &SearchError{Kind: ErrKindProviderUnavailable, Provider: "engine", Temporary: true, Cause: fmt.Errorf("no enabled providers")}
	}
	return SearchResponse{Diagnostics: diag}, &SearchError{Kind: ErrKindNoResults, Provider: "engine", Temporary: false, Cause: fmt.Errorf("all providers exhausted")}
}

func (e *Engine) EnabledProviders() []string {
	out := []string{}
	for _, p := range e.providers {
		if p != nil && p.Enabled() {
			out = append(out, p.Name())
		}
	}
	return out
}

func (e *Engine) emit(ev Event) {
	for _, h := range e.hooks {
		if h != nil {
			h(ev)
		}
	}
}
