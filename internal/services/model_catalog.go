package services

import (
	"context"
	"strings"
	"sync"
)

// ModelCatalog is a configured runtime-neutral model metadata resolver.
type ModelCatalog struct {
	mu    sync.RWMutex
	items map[string]ModelMetadata
}

func NewModelCatalog(items []ModelMetadata) *ModelCatalog {
	catalog := &ModelCatalog{items: make(map[string]ModelMetadata, len(items))}
	for _, item := range items {
		catalog.items[modelKey(item.Provider, item.Model)] = item
	}
	return catalog
}

func (c *ModelCatalog) Resolve(_ context.Context, provider, model string) (ModelMetadata, bool, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	item, ok := c.items[modelKey(provider, model)]
	if !ok && provider != "" {
		item, ok = c.items[modelKey("", model)]
	}
	return item, ok, nil
}

func modelKey(provider, model string) string {
	return strings.ToLower(strings.TrimSpace(provider)) + "\x00" + strings.ToLower(strings.TrimSpace(model))
}
