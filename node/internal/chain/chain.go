// Package chain reads stablecoin transfers from an EVM chain, and does
// nothing else.
//
// Watch-only by construction. There is no signing code here, no key, and no
// method that could ever produce a transaction: the exchange publishes an
// address, watches for USDC arriving at it, and records what it sees. Money
// leaving is a person's act with their own wallet, on a schedule, against a
// queue the exchange keeps. That is the whole design, and it is why this
// package can be small and use nothing but net/http.
package chain

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// The constants, in one place.
const (
	// USDCBase is Circle's USDC on Base mainnet.
	USDCBase = "0x833589fCD6eDb6E08f4c7C32D4f71b54bdA02913"
	// USDCBaseSepolia is the same token on the Base Sepolia testnet.
	USDCBaseSepolia = "0x036CbD53842c5426634e7929541eC2318f3dCF7e"
	// Decimals is how many places USDC carries: one token is 10^6 units.
	Decimals = 6
	// UnitsPerToken is 10^Decimals.
	UnitsPerToken = 1_000_000
	// UnitsPerCent is how many USDC units one USD minor unit is worth at par.
	UnitsPerCent = UnitsPerToken / 100
	// TransferTopic is keccak256("Transfer(address,address,uint256)").
	TransferTopic = "0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef"
	// Confirmations is how many blocks must follow a transfer before it is
	// believed. Base finalises in minutes; twelve blocks is under half of one.
	Confirmations = 12
	// MaxLogRange caps a single eth_getLogs call. Public RPCs refuse wide
	// ranges, and a watcher that has been down for a day must catch up in
	// pieces rather than fail forever on one oversized request.
	MaxLogRange = 2000
)

// Network is a chain the exchange can watch.
type Network struct {
	Name    string `json:"name"`
	ChainID uint64 `json:"chain_id"`
	USDC    string `json:"usdc"`
}

var (
	Base        = Network{Name: "base", ChainID: 8453, USDC: USDCBase}
	BaseSepolia = Network{Name: "base-sepolia", ChainID: 84532, USDC: USDCBaseSepolia}
)

// NetworkNamed looks a network up by the name configuration uses.
func NetworkNamed(name string) (Network, bool) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "base", "base-mainnet":
		return Base, true
	case "base-sepolia", "sepolia":
		return BaseSepolia, true
	}
	return Network{}, false
}

// Transfer is one USDC movement into the watched address.
type Transfer struct {
	TxHash   string `json:"tx"`
	From     string `json:"from"`
	To       string `json:"to"`
	Units    int64  `json:"units"`
	Block    uint64 `json:"block"`
	LogIndex uint64 `json:"log_index"`
}

// Key identifies a transfer uniquely: a transaction can carry several.
func (t Transfer) Key() string { return t.TxHash + ":" + strconv.FormatUint(t.LogIndex, 10) }

// Client reads one chain over JSON-RPC.
type Client struct {
	RPC       string
	Contract  string
	Recipient string
	HTTP      *http.Client
}

// New builds a client for transfers of contract into recipient over rpc.
func New(rpc, contract, recipient string) (*Client, error) {
	if !strings.HasPrefix(rpc, "https://") && !strings.HasPrefix(rpc, "http://") {
		return nil, fmt.Errorf("chain: the RPC URL must be http(s), got %q", rpc)
	}
	if err := ValidAddress(contract); err != nil {
		return nil, fmt.Errorf("chain: contract: %w", err)
	}
	if err := ValidAddress(recipient); err != nil {
		return nil, fmt.Errorf("chain: receiving address: %w", err)
	}
	return &Client{
		RPC: rpc, Contract: Checksum(contract), Recipient: Checksum(recipient),
		HTTP: &http.Client{Timeout: 20 * time.Second},
	}, nil
}

type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int    `json:"id"`
	Method  string `json:"method"`
	Params  []any  `json:"params"`
}

type rpcResponse struct {
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func (c *Client) call(ctx context.Context, method string, params []any, out any) error {
	body, err := json.Marshal(rpcRequest{JSONRPC: "2.0", ID: 1, Method: method, Params: params})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", c.RPC, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	httpc := c.HTTP
	if httpc == nil {
		httpc = http.DefaultClient
	}
	res, err := httpc.Do(req)
	if err != nil {
		return fmt.Errorf("chain: %s: %w", method, err)
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 8<<20))
	if err != nil {
		return err
	}
	if res.StatusCode/100 != 2 {
		return fmt.Errorf("chain: %s: rpc answered %d", method, res.StatusCode)
	}
	var r rpcResponse
	if err := json.Unmarshal(raw, &r); err != nil {
		return fmt.Errorf("chain: %s: unreadable reply: %w", method, err)
	}
	if r.Error != nil {
		return fmt.Errorf("chain: %s: rpc error %d: %s", method, r.Error.Code, r.Error.Message)
	}
	return json.Unmarshal(r.Result, out)
}

// BlockNumber is the chain head as the RPC sees it.
func (c *Client) BlockNumber(ctx context.Context) (uint64, error) {
	var hexN string
	if err := c.call(ctx, "eth_blockNumber", []any{}, &hexN); err != nil {
		return 0, err
	}
	return parseHexUint(hexN)
}

type rpcLog struct {
	Address     string   `json:"address"`
	Topics      []string `json:"topics"`
	Data        string   `json:"data"`
	BlockNumber string   `json:"blockNumber"`
	TxHash      string   `json:"transactionHash"`
	LogIndex    string   `json:"logIndex"`
	Removed     bool     `json:"removed"`
}

// Transfers lists USDC arriving at the recipient in blocks from..to inclusive.
func (c *Client) Transfers(ctx context.Context, from, to uint64) ([]Transfer, error) {
	var out []Transfer
	for lo := from; lo <= to; lo += MaxLogRange {
		hi := lo + MaxLogRange - 1
		if hi > to {
			hi = to
		}
		var logs []rpcLog
		filter := map[string]any{
			"fromBlock": hexUint(lo), "toBlock": hexUint(hi),
			"address": c.Contract,
			"topics":  []any{TransferTopic, nil, topicFor(c.Recipient)},
		}
		if err := c.call(ctx, "eth_getLogs", []any{filter}, &logs); err != nil {
			return nil, err
		}
		for _, l := range logs {
			t, ok := c.transferOf(l)
			if ok {
				out = append(out, t)
			}
		}
		if hi == to {
			break
		}
	}
	return out, nil
}

// transferOf reads a log as a transfer to us, rejecting anything the filter
// should already have excluded: a permissive RPC is not a reason to credit.
func (c *Client) transferOf(l rpcLog) (Transfer, bool) {
	if l.Removed || len(l.Topics) != 3 || !sameAddress(l.Address, c.Contract) ||
		!strings.EqualFold(l.Topics[0], TransferTopic) {
		return Transfer{}, false
	}
	to := addressFromTopic(l.Topics[2])
	if !sameAddress(to, c.Recipient) {
		return Transfer{}, false
	}
	units, err := parseUnits(l.Data)
	if err != nil || units <= 0 {
		return Transfer{}, false
	}
	block, err := parseHexUint(l.BlockNumber)
	if err != nil {
		return Transfer{}, false
	}
	idx, err := parseHexUint(l.LogIndex)
	if err != nil {
		return Transfer{}, false
	}
	return Transfer{
		TxHash: strings.ToLower(l.TxHash), From: addressFromTopic(l.Topics[1]), To: to,
		Units: units, Block: block, LogIndex: idx,
	}, true
}

func hexUint(n uint64) string { return "0x" + strconv.FormatUint(n, 16) }

func parseHexUint(s string) (uint64, error) {
	s = strings.TrimPrefix(strings.TrimPrefix(s, "0x"), "0X")
	if s == "" {
		return 0, fmt.Errorf("chain: empty quantity")
	}
	return strconv.ParseUint(s, 16, 64)
}

// parseUnits reads a uint256 word. Anything beyond int64 is not an amount a
// job could be for and is refused rather than truncated.
func parseUnits(data string) (int64, error) {
	s := strings.TrimPrefix(strings.TrimPrefix(data, "0x"), "0X")
	n, ok := new(big.Int).SetString(s, 16)
	if !ok {
		return 0, fmt.Errorf("chain: unreadable amount %q", data)
	}
	if !n.IsInt64() {
		return 0, fmt.Errorf("chain: amount too large")
	}
	return n.Int64(), nil
}

// Dust is the unique suffix a job's expected amount carries, in USDC units:
// between 1 and 999, so it never reaches a cent and two jobs at the same
// price ask for different amounts.
func Dust(job string) int64 {
	h := sha256.Sum256([]byte("lamdis-usdc-dust:" + job))
	return int64(binary.BigEndian.Uint64(h[:8])%999) + 1
}

// UnitsFor is the exact amount a job asks for: its ceiling in USD minor units
// at par, plus the job's dust.
func UnitsFor(job string, amountMinor int64) int64 {
	return amountMinor*UnitsPerCent + Dust(job)
}

// MinorOf converts USDC units to USD minor units at par, dropping dust.
func MinorOf(units int64) int64 { return units / UnitsPerCent }

// Format writes units as a decimal USDC amount, e.g. 2300 cents + 17 dust as
// "23.000017".
func Format(units int64) string {
	whole, frac := units/UnitsPerToken, units%UnitsPerToken
	return fmt.Sprintf("%d.%06d", whole, frac)
}
