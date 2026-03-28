package kubernetes82550

import (
	"sync"
	"testing"
)

type DockerConfig map[string]DockerConfigEntry

type DockerConfigEntry struct{}

type CachingDockerConfigProvider struct {
	cacheDockerConfig DockerConfig
}

func (d *CachingDockerConfigProvider) Provide() DockerConfig {
	return DockerConfig{}
}

type lazyEcrProvider struct {
	mu sync.Mutex
	// @guarded_by(mu)
	actualProvider *CachingDockerConfigProvider
}

func (p *lazyEcrProvider) LazyProvide() *DockerConfigEntry {
	// p.mu.Lock()
	// defer p.mu.Unlock()

	if p.actualProvider == nil {
		p.actualProvider = &CachingDockerConfigProvider{}
	}
	entry := p.actualProvider.Provide()["0"]
	return &entry
}

func TestKubernetes82550(t *testing.T) {
	provider := &lazyEcrProvider{}
	for i := 0; i < 10; i++ {
		go provider.LazyProvide()
	}
}
