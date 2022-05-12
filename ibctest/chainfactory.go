package ibctest

import (
	"fmt"
	"strings"

	"github.com/strangelove-ventures/ibc-test-framework/chain/penumbra"
	"go.uber.org/zap"

	"github.com/strangelove-ventures/ibc-test-framework/chain/cosmos"
	"github.com/strangelove-ventures/ibc-test-framework/ibc"
)

// ChainFactory describes how to get chains for tests.
// This type currently supports a Pair method,
// but it may be expanded to a Triplet method in the future.
type ChainFactory interface {
	// Pair returns two chains for IBC.
	Pair(testName string) (ibc.Chain, ibc.Chain, error)

	// Name returns a descriptive name of the factory,
	// indicating all of its chains.
	// Depending on how the factory was configured,
	// this may report more than two chains.
	Name() string
}

// BuiltinChainFactory implements ChainFactory to return a fixed set of chains.
// Use NewBuiltinChainFactory to create an instance.
type BuiltinChainFactory struct {
	entries []BuiltinChainFactoryEntry
	log     *zap.Logger
}

// BuiltinChainFactoryEntry describes a chain to be returned from an instance of BuiltinChainFactory.
type BuiltinChainFactoryEntry struct {
	Name          string
	Version       string
	ChainID       string
	NumValidators int
	NumFullNodes  int
}

// NewBuiltinChainFactory returns a BuiltinChainFactory that returns chains defined by entries.
//
// Currently, NewBuiltinChainFactory will panic if entries is not of length 2.
// In the future, this method may allow or require entries to have length 3.
func NewBuiltinChainFactory(entries []BuiltinChainFactoryEntry, logger *zap.Logger) *BuiltinChainFactory {
	return &BuiltinChainFactory{entries: entries, log: logger}
}

// Returns first n chains (pass -1 for all)
func (f *BuiltinChainFactory) GetChains(testName string, n int) ([]ibc.Chain, error) {
	if n > 0 && len(f.entries) < n {
		return nil, fmt.Errorf("received %d entries but required at least %d", len(f.entries), n)
	}
	var chains []ibc.Chain
	for i := 0; (n >= 0 && i < n) || (n < 0 && i < len(f.entries)); i++ {
		e := f.entries[i]
		chain, err := GetChain(testName, e.Name, e.Version, e.ChainID, e.NumValidators, e.NumFullNodes, f.log)
		if err != nil {
			return nil, err
		}
		chains = append(chains, chain)
	}
	return chains, nil
}

// Pair returns two chains to be used in tests that expect exactly two chains.
func (f *BuiltinChainFactory) Pair(testName string) (ibc.Chain, ibc.Chain, error) {
	chains, err := f.GetChains(testName, 2)
	if err != nil {
		return nil, nil, err
	}
	return chains[0], chains[1], nil
}

// Returns all chains
func (f *BuiltinChainFactory) GetAllChains(testName string) ([]ibc.Chain, error) {
	return f.GetChains(testName, -1)
}

func (f *BuiltinChainFactory) Name() string {
	parts := make([]string, len(f.entries))
	for i, e := range f.entries {
		parts[i] = e.Name + "@" + e.Version
	}
	return strings.Join(parts, "+")
}

// CustomChainFactory is a ChainFactory that supports returning chains that are defined by ChainConfig values.
type CustomChainFactory struct {
	entries []CustomChainFactoryEntry
	log     *zap.Logger
}

// CustomChainFactoryEntry describes a chain to be returned by a CustomChainFactory.
type CustomChainFactoryEntry struct {
	Config        ibc.ChainConfig
	NumValidators int
	NumFullNodes  int
}

// NewCustomChainFactory returns a CustomChainFactory that returns chains defined by entries.
//
// Currently, NewCustomChainFactory will panic if entries is not of length 2.
// In the future, this method may allow or require entries to have length 3.
func NewCustomChainFactory(entries []CustomChainFactoryEntry, logger *zap.Logger) *CustomChainFactory {
	return &CustomChainFactory{entries: entries, log: logger}
}

func (e CustomChainFactoryEntry) GetChain(testName string, log *zap.Logger) (ibc.Chain, error) {
	switch e.Config.Type {
	case "cosmos":
		return cosmos.NewCosmosChain(testName, e.Config, e.NumValidators, e.NumFullNodes, log), nil
	case "penumbra":
		return penumbra.NewPenumbraChain(testName, e.Config, e.NumValidators, e.NumFullNodes), nil
	default:
		return nil, fmt.Errorf("only (cosmos, penumbra) type chains are currently supported (got %q)", e.Config.Type)
	}
}

// Returns first n chains (pass -1 for all)
func (f *CustomChainFactory) GetChains(testName string, n int) ([]ibc.Chain, error) {
	if n > 0 && len(f.entries) < n {
		return nil, fmt.Errorf("received %d entries but required at least %d", len(f.entries), n)
	}
	var chains []ibc.Chain
	for i := 0; (n >= 0 && i < n) || (n < 0 && i < len(f.entries)); i++ {
		chain, err := f.entries[i].GetChain(testName, f.log)
		if err != nil {
			return nil, err
		}
		chains = append(chains, chain)
	}
	return chains, nil
}

// Pair returns two chains to be used in tests that expect exactly two chains.
func (f *CustomChainFactory) Pair(testName string) (ibc.Chain, ibc.Chain, error) {
	chains, err := f.GetChains(testName, 2)
	if err != nil {
		return nil, nil, err
	}
	return chains[0], chains[1], nil
}

// Returns all chains
func (f *CustomChainFactory) GetAllChains(testName string) ([]ibc.Chain, error) {
	return f.GetChains(testName, -1)
}

func (f *CustomChainFactory) Name() string {
	parts := make([]string, len(f.entries))
	for i, e := range f.entries {
		parts[i] = e.Config.Name + "@" + e.Config.Images[0].Version
	}
	return strings.Join(parts, "+")
}