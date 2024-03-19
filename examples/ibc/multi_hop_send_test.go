package ibc_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"cosmossdk.io/math"
	transfertypes "github.com/cosmos/ibc-go/v8/modules/apps/transfer/types"
	chantypes "github.com/cosmos/ibc-go/v8/modules/core/04-channel/types"
	"github.com/strangelove-ventures/interchaintest/v8"
	"github.com/strangelove-ventures/interchaintest/v8/chain/cosmos"
	"github.com/strangelove-ventures/interchaintest/v8/ibc"
	"github.com/strangelove-ventures/interchaintest/v8/testreporter"
	"github.com/strangelove-ventures/interchaintest/v8/testutil"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

func TestMultiHopSend(t *testing.T) {

	if testing.Short() {
		t.Skip("skipping in short mode")
	}

	ctx := context.Background()

	numVals := 1
	numFullNodes := 0

	cf := interchaintest.NewBuiltinChainFactory(zaptest.NewLogger(t), []*interchaintest.ChainSpec{
		{Name: "gaia", Version: "v7.0.0", NumValidators: &numVals, NumFullNodes: &numFullNodes, ChainConfig: ibc.ChainConfig{
			GasPrices: "0.0uatom",
		}},
		{Name: "osmosis", Version: "v11.0.0", NumValidators: &numVals, NumFullNodes: &numFullNodes, ChainConfig: ibc.ChainConfig{
			GasPrices: "0.0uosmo",
		}},
		{Name: "juno", Version: "v21.0.0", NumValidators: &numVals, NumFullNodes: &numFullNodes, ChainConfig: ibc.ChainConfig{
			GasPrices: "0.0ujuno",
		}},
	})

	chains, err := cf.Chains(t.Name())
	require.NoError(t, err)
	aChain, bChain, cChain := chains[0], chains[1], chains[2]

	// Relayer Factory
	client, network := interchaintest.DockerSetup(t)
	relayer := interchaintest.NewBuiltinRelayerFactory(ibc.CosmosRly, zaptest.NewLogger(t)).Build(
		t, client, network)

	// Define paths
	abPath := "gaia-osmosis"
	bcPath := "osmosis-juno"

	// Prep Interchain
	ic := interchaintest.NewInterchain().
		AddChain(aChain).
		AddChain(bChain).
		AddChain(cChain).
		AddRelayer(relayer, "relayer").
		AddLink(interchaintest.InterchainLink{
			Chain1:  aChain,
			Chain2:  bChain,
			Relayer: relayer,
			Path:    abPath,
		}).
		AddLink(interchaintest.InterchainLink{
			Chain1:  bChain,
			Chain2:  cChain,
			Relayer: relayer,
			Path:    bcPath,
		})

	// Create log file
	f, err := interchaintest.CreateLogFile(fmt.Sprintf("%d.json", time.Now().Unix()))
	require.NoError(t, err)

	// Reporter/logs
	rep := testreporter.NewReporter(f)
	eRep := rep.RelayerExecReporter(t)

	// Build interchain
	require.NoError(t, ic.Build(ctx, eRep, interchaintest.InterchainBuildOptions{
		TestName:         t.Name(),
		Client:           client,
		NetworkID:        network,
		SkipPathCreation: false,
	},
	),
	)

	// Get channels
	abChannel, err := ibc.GetTransferChannel(ctx, relayer, eRep, aChain.Config().ChainID, bChain.Config().ChainID)
	require.NoError(t, err)
	baChannel := abChannel.Counterparty

	bcChannel, err := ibc.GetTransferChannel(ctx, relayer, eRep, bChain.Config().ChainID, cChain.Config().ChainID)
	require.NoError(t, err)
	cbChannel := bcChannel.Counterparty

	// // Get channels (alternative)
	// channels, err := relayer.GetChannels(ctx, eRep, aChain.Config().ChainID)
	// require.NoError(t, err)
	// abChannel := channels[0]
	// baChannel := abChannel.Counterparty

	// channels, err = relayer.GetChannels(ctx, eRep, cChain.Config().ChainID)
	// require.NoError(t, err)
	// cbChannel := channels[0]
	// bcChannel := cbChannel.Counterparty

	// Create & fund wallets
	fundAmount := math.NewInt(10_000_000)
	users := interchaintest.GetAndFundTestUsers(t, ctx, "default", fundAmount, aChain, bChain, cChain)
	aUser := users[0]
	bUser := users[1]
	cUser := users[2]

	// Ensure gaia user has fund amount
	aBal, err := aChain.GetBalance(ctx, aUser.FormattedAddress(), aChain.Config().Denom)
	require.NoError(t, err)
	require.True(t, aBal.Equal(fundAmount))

	t.Log("Chain A", aChain.Config().ChainID)
	t.Log("Chain B", bChain.Config().ChainID)
	t.Log("Chain C", cChain.Config().ChainID)

	t.Log("A Addr", aUser.FormattedAddress())
	t.Log("B Addr", bUser.FormattedAddress())
	t.Log("C Addr", cUser.FormattedAddress())

	t.Log("AB Channel", abChannel.PortID, abChannel.ChannelID)
	t.Log("BA Channel", baChannel.PortID, baChannel.ChannelID)
	t.Log("BC Channel", bcChannel.PortID, bcChannel.ChannelID)
	t.Log("CB Channel", cbChannel.PortID, cbChannel.ChannelID)

	// Start relayer
	require.NoError(t, relayer.StartRelayer(ctx, eRep, abPath, bcPath))

	// Cleanup
	t.Cleanup(func() {
		_ = relayer.StopRelayer(ctx, eRep)
		_ = ic.Close()
	})

	//
	//
	// == FIRST SEND
	//
	//

	// Send Transaction
	amountToSend := math.NewInt(1_000_000)
	transfer := ibc.WalletAmount{
		Address: bUser.FormattedAddress(),
		Denom:   aChain.Config().Denom,
		Amount:  amountToSend,
	}

	// Chain heights
	aHeight, err := aChain.Height(ctx)
	require.NoError(t, err)
	bHeight, err := bChain.Height(ctx)
	require.NoError(t, err)

	// Send and flush
	tx, err := aChain.SendIBCTransfer(ctx, abChannel.ChannelID, aUser.KeyName(), transfer, ibc.TransferOptions{})
	require.NoError(t, err)
	require.NoError(t, tx.Validate())

	// Poll for MsgRecvPacket on b chain
	_, err = cosmos.PollForMessage[*chantypes.MsgRecvPacket](ctx, bChain.(*cosmos.CosmosChain), cosmos.DefaultEncoding().InterfaceRegistry, bHeight, bHeight+20, nil)
	require.NoError(t, err)

	// Poll for acknowledge on 'a' chain
	_, err = testutil.PollForAck(ctx, aChain.(*cosmos.CosmosChain), aHeight, aHeight+30, tx.Packet)
	require.NoError(t, err)

	// Wait blocks on each chain
	require.NoError(t, testutil.WaitForBlocks(ctx, 3, aChain, bChain))

	// test source wallet has decreased funds
	expected := aBal.Sub(amountToSend)
	aBalNew, err := aChain.GetBalance(ctx, aUser.FormattedAddress(), aChain.Config().Denom)
	require.NoError(t, err)
	require.True(t, aBalNew.Equal(expected))

	// Trace IBC Denom
	baPrefixDenom := transfertypes.GetPrefixedDenom(baChannel.PortID, baChannel.ChannelID, aChain.Config().Denom)
	baDenomTrace := transfertypes.ParseDenomTrace(baPrefixDenom)
	baIBCDenom := baDenomTrace.IBCDenom()

	// Ensure osmosis user has received the amount
	bBal, err := bChain.GetBalance(ctx, bUser.FormattedAddress(), baIBCDenom)
	require.NoError(t, err)
	require.True(t, bBal.Equal(amountToSend))

	//
	//
	// == SECOND SEND
	//
	//

	// Transfer from osmosis to juno
	transfer = ibc.WalletAmount{
		Address: cUser.FormattedAddress(),
		Denom:   baIBCDenom,
		Amount:  amountToSend,
	}

	// Chain heights
	cHeight, err := cChain.Height(ctx)
	require.NoError(t, err)
	bHeight, err = bChain.Height(ctx)
	require.NoError(t, err)

	tx, err = bChain.SendIBCTransfer(ctx, bcChannel.ChannelID, bUser.KeyName(), transfer, ibc.TransferOptions{})
	require.NoError(t, err)
	require.NoError(t, tx.Validate())

	// Poll for MsgRecvPacket on c chain
	_, err = cosmos.PollForMessage[*chantypes.MsgRecvPacket](ctx, cChain.(*cosmos.CosmosChain), cosmos.DefaultEncoding().InterfaceRegistry, cHeight, cHeight+20, nil)
	require.NoError(t, err)

	// Poll for acknowledge on b chain
	_, err = testutil.PollForAck(ctx, bChain.(*cosmos.CosmosChain), bHeight, bHeight+30, tx.Packet)
	require.NoError(t, err)

	// Wait blocks on each chain
	require.NoError(t, testutil.WaitForBlocks(ctx, 3, bChain, cChain))

	// Test source wallet has decreased funds
	expected = bBal.Sub(amountToSend)
	bBalNew, err := bChain.GetBalance(ctx, bUser.FormattedAddress(), baIBCDenom)
	t.Log("Osmo New Bal", bBal)
	require.NoError(t, err)
	require.True(t, bBalNew.Equal(expected))

	// Trace IBC Denom
	cbDenomTrace := transfertypes.ParseDenomTrace(transfertypes.GetPrefixedDenom(cbChannel.PortID, cbChannel.ChannelID, baPrefixDenom))
	cbIBCDenom := cbDenomTrace.IBCDenom()

	// Ensure juno user has received the amount
	junoUserBal, err := cChain.GetBalance(ctx, cUser.FormattedAddress(), cbIBCDenom)
	require.NoError(t, err)
	require.True(t, junoUserBal.Equal(amountToSend))
}
