package upstream

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"sync"

	"github.com/erpc/erpc/common"
)

var drpcNetworkNames = map[int64]string{
	1:           "ethereum",
	10:          "optimism",
	100:         "gnosis",
	1001:        "klaytn-baobab",
	10200:       "gnosis-chiado",
	108:         "thundercore",
	1088:        "metis",
	10888:       "gameswift-testnet",
	1100:        "dymension",
	1101:        "polygon-zkevm",
	1111:        "wemix",
	111188:      "real",
	1112:        "wemix-testnet",
	1115:        "core-testnet",
	11155111:    "sepolia",
	11155420:    "optimism-sepolia",
	1116:        "core",
	11235:       "haqq",
	1135:        "lisk",
	122:         "fuse",
	123420111:   "opcelestia-raspberry-testnet",
	1284:        "moonbeam",
	1285:        "moonriver",
	1287:        "moonbase-alpha",
	1313161554:  "aurora",
	1313161555:  "aurora-testnet",
	13371:       "immutable-zkevm",
	13473:       "immutable-zkevm-testnet",
	137:         "polygon",
	1666600000:  "harmony-0",
	1666600001:  "harmony-1",
	167000:      "taiko",
	167009:      "taiko-hekla",
	168587773:   "blast-sepolia",
	169:         "manta-pacific",
	17000:       "holesky",
	1740:        "metall2-testnet",
	1750:        "metall2",
	18:          "thundercore-testnet",
	1829:        "playnance",
	199:         "bittorrent",
	2020:        "ronin",
	2039:        "alephzero-sepolia",
	204:         "opbnb",
	2221:        "kava-testnet",
	2222:        "kava",
	2358:        "kroma-sepolia",
	2442:        "polygon-zkevm-cardona",
	245022926:   "neon-evm-devnet",
	245022934:   "neon-evm",
	2494104990:  "tron-shasta",
	25:          "cronos",
	250:         "fantom",
	252:         "fraxtal",
	2522:        "fraxtal-testnet",
	255:         "kroma",
	288:         "boba-eth",
	30:          "rootstock",
	300:         "zksync-sepolia",
	31:          "rootstock-testnet",
	314:         "filecoin",
	314159:      "filecoin-calibration",
	324:         "zksync",
	338:         "cronos-testnet",
	3441006:     "manta-pacific-sepolia",
	34443:       "mode",
	3456:        "goat-testnet",
	40:          "telos",
	4002:        "fantom-testnet",
	41:          "telos-testnet",
	41455:       "alephzero",
	4202:        "lisk-sepolia",
	42161:       "arbitrum",
	421614:      "arbitrum-sepolia",
	42170:       "arbitrum-nova",
	42220:       "celo",
	43113:       "avalanche-fuji",
	43114:       "avalanche",
	44787:       "celo-alfajores",
	48899:       "zircuit-testnet",
	48900:       "zircuit-mainnet",
	5000:        "mantle",
	5003:        "mantle-sepolia",
	534351:      "scroll-sepolia",
	534352:      "scroll",
	54211:       "haqq-testnet",
	56:          "bsc",
	5611:        "opbnb-testnet",
	56288:       "boba-bnb",
	59141:       "linea-sepolia",
	59144:       "linea",
	60808:       "bob",
	6398:        "everclear-sepolia",
	65:          "oktc-testnet",
	656476:      "open-campus-codex-sepolia",
	66:          "oktc",
	7000:        "zeta-chain",
	7001:        "zeta-chain-testnet",
	728126428:   "tron",
	7777777:     "zora",
	80002:       "polygon-amoy",
	80084:       "bartio",
	808813:      "bob-testnet",
	81457:       "blast",
	8217:        "klaytn",
	8453:        "base",
	84532:       "base-sepolia",
	88153591557: "arb-blueberry-testnet",
	9000:        "evmos-testnet",
	9001:        "evmos",
	919:         "mode-testnet",
	94204209:    "polygon-blackberry-testnet",
	97:          "bsc-testnet",
	999999999:   "zora-sepolia",
}

type DrpcHttpJsonRpcClient struct {
	upstream *Upstream
	apiKey   string
	clients  map[string]HttpJsonRpcClient
	mu       sync.RWMutex
}

func NewDrpcHttpJsonRpcClient(pu *Upstream, parsedUrl *url.URL) (HttpJsonRpcClient, error) {
	if !strings.HasSuffix(parsedUrl.Scheme, "drpc") {
		return nil, fmt.Errorf("invalid DRPC URL scheme: %s", parsedUrl.Scheme)
	}

	apiKey := parsedUrl.Host
	if apiKey == "" {
		return nil, fmt.Errorf("missing DRPC API key in URL")
	}

	return &DrpcHttpJsonRpcClient{
		upstream: pu,
		apiKey:   apiKey,
		clients:  make(map[string]HttpJsonRpcClient),
	}, nil
}

func (c *DrpcHttpJsonRpcClient) GetType() ClientType {
	return ClientTypeDrpcHttpJsonRpc
}

func (c *DrpcHttpJsonRpcClient) SupportsNetwork(networkId string) (bool, error) {
	if !strings.HasPrefix(networkId, "evm:") {
		return false, nil
	}

	chainId, err := strconv.ParseInt(networkId[4:], 10, 64)
	if err != nil {
		return false, err
	}

	_, ok := drpcNetworkNames[chainId]
	return ok, nil
}

func (c *DrpcHttpJsonRpcClient) getOrCreateClient(network common.Network) (HttpJsonRpcClient, error) {
	networkID := network.Id()
	c.mu.RLock()
	client, exists := c.clients[networkID]
	c.mu.RUnlock()

	if exists {
		return client, nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Double-check to ensure another goroutine hasn't created the client
	if client, exists := c.clients[networkID]; exists {
		return client, nil
	}

	if network.Architecture() != common.ArchitectureEvm {
		return nil, fmt.Errorf("unsupported network architecture for DRPC client: %s", network.Architecture())
	}

	chainID, err := network.EvmChainId()
	if err != nil {
		return nil, err
	}

	netName, ok := drpcNetworkNames[chainID]
	if !ok {
		return nil, fmt.Errorf("unsupported network chain ID for DRPC: %d", chainID)
	}

	drpcURL := fmt.Sprintf("https://lb.drpc.org/ogrpc?network=%s&dkey=%s", netName, c.apiKey)
	parsedURL, err := url.Parse(drpcURL)
	if err != nil {
		return nil, err
	}

	client, err = NewGenericHttpJsonRpcClient(&c.upstream.Logger, c.upstream, parsedURL)
	if err != nil {
		return nil, err
	}

	c.clients[networkID] = client
	return client, nil
}

func (c *DrpcHttpJsonRpcClient) SendRequest(ctx context.Context, req *common.NormalizedRequest) (*common.NormalizedResponse, error) {
	network := req.Network()
	if network == nil {
		return nil, fmt.Errorf("network information is missing in the request")
	}

	client, err := c.getOrCreateClient(network)
	if err != nil {
		return nil, err
	}

	return client.SendRequest(ctx, req)
}