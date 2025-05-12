package solana

import (
	"context"
	"crypto/sha256"
	"encoding/json" // Standard errors package
	"errors"        // Standard errors package for errors.As
	"fmt"
	"io"
	"net/http"
	"os"            // Added for file operations
	"path/filepath" // Added for path manipulation
	"strings"
	"sync"
	"time"

	bin "github.com/gagliardetto/binary" // Explicitly import jsonrpc
	tokenmetadata "github.com/gagliardetto/metaplex-go/clients/token-metadata"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	jrpc "github.com/gagliardetto/solana-go/rpc/jsonrpc" // For jrpc.RPCError
	"github.com/hunterwarburton/ya8hoda/internal/logger"
)

// DefaultMainnetEndpoint is the public RPC endpoint for Solana mainnet-beta.
const DefaultMainnetEndpoint = "https://api.mainnet-beta.solana.com/"

// MetaplexTokenMetadataProgramID is the program ID for the Metaplex Token Metadata program.
const MetaplexTokenMetadataProgramID = "metaqbxxUerdq28cj1RbAWkYQm3ybzjb6a8bt518x1s"

// NEW: Define cache path
const solanaCacheDir = "/app/data/solana/cache"

var metaplexProgramID = solana.MustPublicKeyFromBase58(MetaplexTokenMetadataProgramID)

// Client uses the solana-go SDK's RPC client.
type Client struct {
	rpcClient  *rpc.Client
	httpClient *http.Client // Retained for fetching off-chain JSON metadata
}

// NewClient creates a new RPC client pointing to the specified endpoint.
// If endpoint is an empty string DefaultMainnetEndpoint is used.
func NewClient(endpoint string) *Client {
	startTime := time.Now()
	defer func() {
		logger.ToolDebug("NewClient took %v to initialize", time.Since(startTime))
	}()

	if endpoint == "" {
		endpoint = DefaultMainnetEndpoint
	}

	// NEW: Ensure cache directory exists
	if err := os.MkdirAll(solanaCacheDir, 0755); err != nil {
		logger.ToolWarn("Failed to create Solana cache directory at %s: %v. Disk caching will fail.", solanaCacheDir, err)
		// Depending on requirements, you might want to panic or return an error here
	}

	return &Client{
		rpcClient: rpc.New(endpoint),
		httpClient: &http.Client{ // Standard HTTP client for off-chain data
			Timeout: 30 * time.Second,
		},
	}
}

// ------------------- package-level token metadata cache (NEW: For Metaplex) --------------------
// This will be used for caching rich token metadata fetched via Metaplex.
// For now, the old token list logic is removed. The new logic will be added in a subsequent step.

// TODO: Define a new struct for rich token metadata (e.g., FullTokenMetadata)
// TODO: Implement a new caching mechanism (e.g., sync.Map or mutex-protected map) -> Done with metaplexCache
// TODO: Implement fetchAndCacheFullTokenMetadata function -> This will be part of GetTokenMetadata

// (Old token list related variables and functions like tokenMetaOnce, tokenMetaErr, tokenMetaMap,
// loadTokenList, and getTokenMeta are removed as they will be replaced by Metaplex logic)

// MetaplexOffChainJSON represents the structure of the JSON file typically hosted at MetaplexData.Uri.
// We only include fields we're interested in.
type MetaplexOffChainJSON struct {
	Name        string                 `json:"name"`
	Symbol      string                 `json:"symbol"`
	Description string                 `json:"description"`
	Image       string                 `json:"image"` // This is often the logo URI
	Attributes  []MetaplexAttribute    `json:"attributes,omitempty"`
	Extensions  map[string]interface{} `json:"extensions,omitempty"`
}

// MetaplexAttribute is a common structure for attributes in off-chain metadata.
type MetaplexAttribute struct {
	TraitType string `json:"trait_type"`
	Value     string `json:"value"`
}

// UnmarshalJSON implements a custom unmarshaller for MetaplexAttribute
// to handle cases where the "value" field might be a number or boolean instead of a string.
func (m *MetaplexAttribute) UnmarshalJSON(data []byte) error {
	// Use an auxiliary type to avoid recursion during unmarshalling
	type AuxMetaplexAttribute struct {
		TraitType string      `json:"trait_type"`
		Value     interface{} `json:"value"`
	}

	var aux AuxMetaplexAttribute
	if err := json.Unmarshal(data, &aux); err != nil {
		return fmt.Errorf("failed to unmarshal to aux MetaplexAttribute: %w", err)
	}

	m.TraitType = aux.TraitType

	switch v := aux.Value.(type) {
	case string:
		m.Value = v
	case float64: // JSON numbers are unmarshalled as float64
		// Convert number to string; use strconv for more control if needed,
		// but fmt.Sprintf is often sufficient.
		m.Value = fmt.Sprintf("%g", v) // %g is often good for numbers
	case bool:
		m.Value = fmt.Sprintf("%t", v) // Convert bool to string
	case nil:
		m.Value = "" // Handle null as empty string or as appropriate
	default:
		// For other unexpected types, either return an error or attempt a generic conversion.
		// Attempting a generic conversion might hide issues, so logging could be useful here.
		logger.ToolWarn("Unexpected type for MetaplexAttribute.Value: %T, converting with %%v", v)
		m.Value = fmt.Sprintf("%v", v)
		// Alternatively, to be stricter:
		// return fmt.Errorf("unexpected type %T for MetaplexAttribute.Value: %v", v, v)
	}

	return nil
}

// Error type constants for permanent metadata fetch failures
const (
	// On-chain error types
	ErrorTypeOnchainMetadataNotFound  = "onchain_metadata_not_found"
	ErrorTypeOnchainDeserializeFailed = "onchain_deserialize_failed"

	// Off-chain error types
	ErrorTypeOffchainURIInvalidChars = "offchain_uri_invalid_chars"
	ErrorTypeOffchainDNSLookupFailed = "offchain_dns_lookup_failed"
	ErrorTypeOffchainFetchTimeout    = "offchain_fetch_timeout"
	ErrorTypeOffchainHTTP403         = "offchain_http_403"
	ErrorTypeOffchainHTTP404         = "offchain_http_404"
	ErrorTypeOffchainURIReturnsHTML  = "offchain_uri_returns_html"
	ErrorTypeOffchainJSONMalformed   = "offchain_json_malformed"
)

// FullTokenInfo is what we'll ultimately return/cache, combining on-chain and off-chain data.
// This is what GetTokenMetadata will aim to produce.
type FullTokenInfo struct {
	MintAddress   string
	OnChainName   string
	OnChainSymbol string
	OffChainURI   string

	// From Off-chain JSON
	ResolvedName        string
	ResolvedSymbol      string
	ResolvedDescription string
	ResolvedImageURI    string
	Attributes          []MetaplexAttribute
	Extensions          map[string]interface{}

	// NEW: Error tracking fields
	IsPermanentlyBad bool   `json:"is_permanently_bad,omitempty"`
	ErrorType        string `json:"error_type,omitempty"`
	ErrorMessage     string `json:"error_message,omitempty"`
}

// deriveMetaplexMetadataPDA derives the Metaplex Token Metadata PDA for a given mint.
func deriveMetaplexMetadataPDA(mint solana.PublicKey) (solana.PublicKey, error) {
	pda, _, err := solana.FindProgramAddress(
		[][]byte{
			[]byte("metadata"),
			metaplexProgramID.Bytes(),
			mint.Bytes(),
		},
		metaplexProgramID,
	)
	if err != nil {
		return solana.PublicKey{}, fmt.Errorf("failed to find Metaplex metadata PDA: %w", err)
	}
	return pda, nil
}

// NEW: loadFullTokenInfoFromCache helper function
// loadFullTokenInfoFromCache tries to load FullTokenInfo from disk cache.
// Returns the info and nil error on cache hit and successful unmarshal.
// Returns nil, nil if cache miss (file not found).
// Returns nil, error for other errors (e.g., read error, unmarshal error, stat error other than NotExist).
func (c *Client) loadFullTokenInfoFromCache(mintAddress string) (*FullTokenInfo, error) {
	cacheFilePath := filepath.Join(solanaCacheDir, mintAddress+".json")

	fileInfo, statErr := os.Stat(cacheFilePath)
	if statErr != nil {
		if os.IsNotExist(statErr) {
			logger.ToolDebug("Disk cache miss (pre-check) for Metaplex metadata: %s (file not found at %s).", mintAddress, cacheFilePath)
			return nil, nil // Cache miss, not an error
		}
		// Some other error occurred when checking for the file (e.g., permissions)
		logger.ToolWarn("Error stating cache file %s for mint %s during pre-check: %v.", cacheFilePath, mintAddress, statErr)
		return nil, fmt.Errorf("error stating cache file %s: %w", cacheFilePath, statErr)
	}

	// Check if the found item is a directory, which would be unexpected
	if fileInfo.IsDir() {
		logger.ToolWarn("Cache path %s for mint %s is a directory, not a file. Cache will be ignored.", cacheFilePath, mintAddress)
		return nil, fmt.Errorf("cache path %s is a directory", cacheFilePath)
	}

	// Cache file exists, try to load it
	cachedData, readErr := os.ReadFile(cacheFilePath)
	if readErr != nil {
		logger.ToolWarn("Failed to read cached Metaplex metadata for %s from %s: %v. Cache will be ignored.", mintAddress, cacheFilePath, readErr)
		return nil, fmt.Errorf("failed to read cache file %s: %w", cacheFilePath, readErr)
	}

	var fullInfo FullTokenInfo
	if unmarshalErr := json.Unmarshal(cachedData, &fullInfo); unmarshalErr != nil {
		logger.ToolWarn("Failed to unmarshal cached Metaplex metadata for %s from %s: %v. Cache will be ignored.", mintAddress, cacheFilePath, unmarshalErr)
		return nil, fmt.Errorf("failed to unmarshal cache file %s: %w", cacheFilePath, unmarshalErr)
	}

	logger.ToolDebug("Disk cache hit (pre-check) for Metaplex metadata: %s from %s", mintAddress, cacheFilePath)
	return &fullInfo, nil
}

// writeFullTokenInfoToCache writes the given FullTokenInfo to disk cache.
// It returns any error encountered during the write operation.
func (c *Client) writeFullTokenInfoToCache(mintAddress string, fullInfo *FullTokenInfo) error {
	if fullInfo == nil {
		return fmt.Errorf("cannot cache nil FullTokenInfo")
	}

	cacheFilePath := filepath.Join(solanaCacheDir, mintAddress+".json")
	jsonData, err := json.MarshalIndent(fullInfo, "", "  ")
	if err != nil {
		logger.ToolWarn("Failed to marshal FullTokenInfo for mint %s for disk caching: %v", mintAddress, err)
		return err
	}

	if err := os.WriteFile(cacheFilePath, jsonData, 0644); err != nil {
		logger.ToolWarn("Failed to write Metaplex metadata to disk cache for %s at %s: %v", mintAddress, cacheFilePath, err)
		return err
	}

	logger.ToolDebug("Stored Metaplex metadata in disk cache for: %s at %s", mintAddress, cacheFilePath)
	return nil
}

// GetTokenMetadata fetches, decodes, and enriches token metadata for a given mint.
// It gets on-chain data from Metaplex PDA and then fetches off-chain JSON from the URI.
func (c *Client) GetTokenMetadata(ctx context.Context, mintAddress string) (*FullTokenInfo, error) {
	// Check disk cache first
	cachedInfo, err := c.loadFullTokenInfoFromCache(mintAddress)
	if err == nil && cachedInfo != nil {
		// If we have a cached entry (whether good or permanently bad), use it
		if cachedInfo.IsPermanentlyBad {
			logger.ToolDebug("Using cached permanent error for mint %s: [%s] %s",
				mintAddress, cachedInfo.ErrorType, cachedInfo.ErrorMessage)
		} else {
			logger.ToolDebug("Using cached metadata for mint %s", mintAddress)
		}
		return cachedInfo, nil
	}

	logger.ToolDebug("Disk cache miss or error for Metaplex metadata: %s. Fetching from network.", mintAddress)

	mintPk, err := solana.PublicKeyFromBase58(mintAddress)
	if err != nil {
		return nil, fmt.Errorf("invalid mint address '%s': %w", mintAddress, err)
	}

	metadataPDA, err := deriveMetaplexMetadataPDA(mintPk)
	if err != nil {
		return nil, err // error already descriptive
	}

	accountInfo, err := c.rpcClient.GetAccountInfo(ctx, metadataPDA)
	if err != nil {
		var rpcErr *jrpc.RPCError
		if errors.As(err, &rpcErr) {
			// Check if this is a "not found" error
			if strings.Contains(err.Error(), "not found") {
				fullInfo := &FullTokenInfo{
					MintAddress:      mintAddress,
					IsPermanentlyBad: true,
					ErrorType:        ErrorTypeOnchainMetadataNotFound,
					ErrorMessage:     fmt.Sprintf("Metaplex metadata account %s not found", metadataPDA),
				}
				if cacheErr := c.writeFullTokenInfoToCache(mintAddress, fullInfo); cacheErr != nil {
					logger.ToolWarn("Failed to cache permanent error for mint %s: %v", mintAddress, cacheErr)
				}
				return fullInfo, nil
			}
			// For other RPC errors (like rate limiting), return the error for retry
			return nil, fmt.Errorf("RPC error fetching Metaplex metadata account %s for mint %s: %w", metadataPDA, mintAddress, err)
		}
	}
	if accountInfo == nil || accountInfo.Value == nil {
		fullInfo := &FullTokenInfo{
			MintAddress:      mintAddress,
			IsPermanentlyBad: true,
			ErrorType:        ErrorTypeOnchainMetadataNotFound,
			ErrorMessage:     fmt.Sprintf("Metaplex metadata account %s not found or empty", metadataPDA),
		}
		if cacheErr := c.writeFullTokenInfoToCache(mintAddress, fullInfo); cacheErr != nil {
			logger.ToolWarn("Failed to cache permanent error for mint %s: %v", mintAddress, cacheErr)
		}
		return fullInfo, nil
	}

	if accountInfo.Value.Owner != metaplexProgramID {
		fullInfo := &FullTokenInfo{
			MintAddress:      mintAddress,
			IsPermanentlyBad: true,
			ErrorType:        ErrorTypeOnchainMetadataNotFound,
			ErrorMessage:     fmt.Sprintf("Metaplex metadata account %s has wrong owner: %s", metadataPDA, accountInfo.Value.Owner),
		}
		if cacheErr := c.writeFullTokenInfoToCache(mintAddress, fullInfo); cacheErr != nil {
			logger.ToolWarn("Failed to cache permanent error for mint %s: %v", mintAddress, cacheErr)
		}
		return fullInfo, nil
	}

	// Deserialize the on-chain metadata using metaplex-go SDK
	accountDataBytes := accountInfo.Value.Data.GetBinary()
	if accountDataBytes == nil {
		fullInfo := &FullTokenInfo{
			MintAddress:      mintAddress,
			IsPermanentlyBad: true,
			ErrorType:        ErrorTypeOnchainDeserializeFailed,
			ErrorMessage:     fmt.Sprintf("Account data is nil for Metaplex metadata PDA %s", metadataPDA),
		}
		if cacheErr := c.writeFullTokenInfoToCache(mintAddress, fullInfo); cacheErr != nil {
			logger.ToolWarn("Failed to cache permanent error for mint %s: %v", mintAddress, cacheErr)
		}
		return fullInfo, nil
	}

	var onChainMeta tokenmetadata.Metadata
	decoder := bin.NewBorshDecoder(accountDataBytes)
	err = decoder.Decode(&onChainMeta)
	if err != nil {
		fullInfo := &FullTokenInfo{
			MintAddress:      mintAddress,
			IsPermanentlyBad: true,
			ErrorType:        ErrorTypeOnchainDeserializeFailed,
			ErrorMessage:     fmt.Sprintf("Failed to deserialize on-chain Metaplex metadata: %v", err),
		}
		if cacheErr := c.writeFullTokenInfoToCache(mintAddress, fullInfo); cacheErr != nil {
			logger.ToolWarn("Failed to cache permanent error for mint %s: %v", mintAddress, cacheErr)
		}
		return fullInfo, nil
	}

	// Trim null characters from URI, Name, and Symbol as they are often padded in on-chain data
	uri := strings.TrimRight(onChainMeta.Data.Uri, "\x00")
	name := strings.TrimRight(onChainMeta.Data.Name, "\x00")
	symbol := strings.TrimRight(onChainMeta.Data.Symbol, "\x00")

	fullInfo := &FullTokenInfo{
		MintAddress:   mintAddress,
		OnChainName:   name,
		OnChainSymbol: symbol,
		OffChainURI:   uri,
	}

	// Fetch and parse off-chain JSON metadata if URI exists
	if uri != "" {
		cleanedURI := strings.TrimSpace(uri)
		if cleanedURI == "" {
			return fullInfo, nil
		}

		httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, cleanedURI, nil)
		if err != nil {
			if strings.Contains(err.Error(), "invalid control character") {
				fullInfo.IsPermanentlyBad = true
				fullInfo.ErrorType = ErrorTypeOffchainURIInvalidChars
				fullInfo.ErrorMessage = fmt.Sprintf("Invalid control characters in URI: %v", err)
				if cacheErr := c.writeFullTokenInfoToCache(mintAddress, fullInfo); cacheErr != nil {
					logger.ToolWarn("Failed to cache permanent error for mint %s: %v", mintAddress, cacheErr)
				}
				return fullInfo, nil
			}
			logger.ToolWarn("Failed to create HTTP request for off-chain metadata for mint %s (URI: %s): %v", mintAddress, cleanedURI, err)
			return fullInfo, nil
		}

		httpResp, err := c.httpClient.Do(httpReq)
		if err != nil {
			if strings.Contains(err.Error(), "no such host") {
				fullInfo.IsPermanentlyBad = true
				fullInfo.ErrorType = ErrorTypeOffchainDNSLookupFailed
				fullInfo.ErrorMessage = fmt.Sprintf("DNS lookup failed for URI host: %v", err)
				if cacheErr := c.writeFullTokenInfoToCache(mintAddress, fullInfo); cacheErr != nil {
					logger.ToolWarn("Failed to cache permanent error for mint %s: %v", mintAddress, cacheErr)
				}
				return fullInfo, nil
			} else if strings.Contains(err.Error(), "context deadline exceeded") {
				fullInfo.IsPermanentlyBad = true
				fullInfo.ErrorType = ErrorTypeOffchainFetchTimeout
				fullInfo.ErrorMessage = fmt.Sprintf("Timeout fetching off-chain metadata: %v", err)
				if cacheErr := c.writeFullTokenInfoToCache(mintAddress, fullInfo); cacheErr != nil {
					logger.ToolWarn("Failed to cache permanent error for mint %s: %v", mintAddress, cacheErr)
				}
				return fullInfo, nil
			}
			logger.ToolWarn("Failed to fetch off-chain metadata for mint %s (URI: %s): %v", mintAddress, cleanedURI, err)
			return fullInfo, nil
		}
		defer httpResp.Body.Close()

		if httpResp.StatusCode == http.StatusForbidden {
			fullInfo.IsPermanentlyBad = true
			fullInfo.ErrorType = ErrorTypeOffchainHTTP403
			fullInfo.ErrorMessage = fmt.Sprintf("HTTP 403 Forbidden response from URI: %s", cleanedURI)
			if cacheErr := c.writeFullTokenInfoToCache(mintAddress, fullInfo); cacheErr != nil {
				logger.ToolWarn("Failed to cache permanent error for mint %s: %v", mintAddress, cacheErr)
			}
			return fullInfo, nil
		} else if httpResp.StatusCode == http.StatusNotFound {
			fullInfo.IsPermanentlyBad = true
			fullInfo.ErrorType = ErrorTypeOffchainHTTP404
			fullInfo.ErrorMessage = fmt.Sprintf("HTTP 404 Not Found response from URI: %s", cleanedURI)
			if cacheErr := c.writeFullTokenInfoToCache(mintAddress, fullInfo); cacheErr != nil {
				logger.ToolWarn("Failed to cache permanent error for mint %s: %v", mintAddress, cacheErr)
			}
			return fullInfo, nil
		} else if httpResp.StatusCode != http.StatusOK {
			logger.ToolWarn("HTTP error fetching off-chain metadata for mint %s (URI: %s): status %d", mintAddress, cleanedURI, httpResp.StatusCode)
			return fullInfo, nil
		}

		body, err := io.ReadAll(httpResp.Body)
		if err != nil {
			logger.ToolWarn("Failed to read off-chain metadata body for mint %s (URI: %s): %v", mintAddress, cleanedURI, err)
			return fullInfo, nil
		}

		// Check if the response looks like HTML
		if strings.Contains(string(body), "<!DOCTYPE html>") || strings.Contains(string(body), "<html>") {
			fullInfo.IsPermanentlyBad = true
			fullInfo.ErrorType = ErrorTypeOffchainURIReturnsHTML
			fullInfo.ErrorMessage = fmt.Sprintf("URI returns HTML instead of JSON: %s", cleanedURI)
			if cacheErr := c.writeFullTokenInfoToCache(mintAddress, fullInfo); cacheErr != nil {
				logger.ToolWarn("Failed to cache permanent error for mint %s: %v", mintAddress, cacheErr)
			}
			return fullInfo, nil
		}

		var offChainJSON MetaplexOffChainJSON
		if err := json.Unmarshal(body, &offChainJSON); err != nil {
			cleanedBody := strings.ReplaceAll(string(body), "\n", "\\n")
			if errRetry := json.Unmarshal([]byte(cleanedBody), &offChainJSON); errRetry != nil {
				fullInfo.IsPermanentlyBad = true
				fullInfo.ErrorType = ErrorTypeOffchainJSONMalformed
				fullInfo.ErrorMessage = fmt.Sprintf("Failed to unmarshal JSON (even after newline fix): %v", errRetry)
				if cacheErr := c.writeFullTokenInfoToCache(mintAddress, fullInfo); cacheErr != nil {
					logger.ToolWarn("Failed to cache permanent error for mint %s: %v", mintAddress, cacheErr)
				}
				return fullInfo, nil
			}
		}

		fullInfo.ResolvedName = strings.TrimRight(offChainJSON.Name, "\x00")
		fullInfo.ResolvedSymbol = strings.TrimRight(offChainJSON.Symbol, "\x00")
		fullInfo.ResolvedDescription = strings.TrimRight(offChainJSON.Description, "\x00")
		fullInfo.ResolvedImageURI = offChainJSON.Image
		fullInfo.Attributes = offChainJSON.Attributes
		fullInfo.Extensions = offChainJSON.Extensions
	}

	// Cache successful result
	if cacheErr := c.writeFullTokenInfoToCache(mintAddress, fullInfo); cacheErr != nil {
		logger.ToolWarn("Failed to cache successful metadata for mint %s: %v", mintAddress, cacheErr)
	}

	return fullInfo, nil
}

// ------------------- Token Balances -------------------

// TokenAccount represents a single SPL Token account with its mint, and amount.
// Metadata like symbol and name will be populated from the new Metaplex-based fetching.
type TokenAccount struct {
	Mint         string  `json:"mint"`
	Amount       float64 `json:"amount"`
	Decimals     int     `json:"decimals"`
	TokenAddress string  `json:"token_address"` // The specific token account address

	// Fields to be populated by Metaplex metadata
	Symbol      string                 `json:"symbol,omitempty"`
	Name        string                 `json:"name,omitempty"`
	LogoURI     string                 `json:"logo_uri,omitempty"`
	Description string                 `json:"description,omitempty"`
	Attributes  []MetaplexAttribute    `json:"attributes,omitempty"`
	Extensions  map[string]interface{} `json:"extensions,omitempty"`
}

// GetTokenBalances returns aggregated SPL token balances for the given owner
// (wallet) address. The returned slice contains one entry per mint with the
// UI amount already adjusted for decimals, and will be enriched with token metadata.
func (c *Client) GetTokenBalances(ctx context.Context, ownerPubkeyStr string) ([]TokenAccount, error) {
	ownerPk, err := solana.PublicKeyFromBase58(ownerPubkeyStr)
	if err != nil {
		return nil, fmt.Errorf("invalid owner pubkey '%s': %w", ownerPubkeyStr, err)
	}

	tokenProgramID := solana.TokenProgramID // from solana-go SDK

	config := &rpc.GetTokenAccountsConfig{
		ProgramId: &tokenProgramID,
	}
	opts := &rpc.GetTokenAccountsOpts{
		Encoding: solana.EncodingJSONParsed,
	}
	accts, err := c.rpcClient.GetTokenAccountsByOwner(ctx, ownerPk, config, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to get token accounts by owner: %w", err)
	}

	// Aggregate by mint
	aggregatedAccounts := make(map[string]TokenAccount)

	for _, rawAcct := range accts.Value {
		tokenAcctAddr := rawAcct.Pubkey.String()

		// GetRawJSON() should give us the bytes of the JSON structure within Data.
		rawJSONData := rawAcct.Account.Data.GetRawJSON()
		if rawJSONData == nil {
			// logger.Warn("GetRawJSON() returned nil for token account %s", tokenAcctAddr)
			continue
		}

		var accountDataParent map[string]interface{}
		if err := json.Unmarshal(rawJSONData, &accountDataParent); err != nil {
			// logger.Warn("Failed to unmarshal raw JSON data for token account %s: %v", tokenAcctAddr, err)
			continue
		}

		// The structure is: accountDataParent -> "parsed" -> "info"
		parsedField, ok := accountDataParent["parsed"].(map[string]interface{})
		if !ok {
			// logger.Warn("Failed to find or assert 'parsed' field in account data for %s. Data: %s", tokenAcctAddr, string(rawJSONData))
			continue
		}

		info, ok := parsedField["info"].(map[string]interface{})
		if !ok {
			// logger.Warn("Failed to find or assert 'info' field in parsed account data for %s. Parsed: %+v", tokenAcctAddr, parsedField)
			continue
		}

		mint, ok := info["mint"].(string)
		if !ok || mint == "" {
			// logger.Debug("Missing mint for token account %s", tokenAcctAddr)
			continue
		}
		tokenAmountMap, ok := info["tokenAmount"].(map[string]interface{})
		if !ok {
			// logger.Debug("Missing tokenAmount for mint %s, account %s", mint, tokenAcctAddr)
			continue
		}

		uiAmount, uiAmountOk := tokenAmountMap["uiAmount"].(float64)
		decimals, decimalsOk := tokenAmountMap["decimals"].(float64) // json.Unmarshal puts numbers into float64
		if !uiAmountOk || !decimalsOk {
			// logger.Debug("Missing uiAmount or decimals for mint %s, account %s", mint, tokenAcctAddr)
			continue
		}

		current, exists := aggregatedAccounts[mint]
		if exists {
			current.Amount += uiAmount
			// Update tokenAddress if the current one is empty, or prefer shorter (usually non-ATA) ones if desired.
			// This is a minor heuristic; for most cases, any valid token account address for the mint is fine.
			if current.TokenAddress == "" || len(tokenAcctAddr) < len(current.TokenAddress) {
				current.TokenAddress = tokenAcctAddr
			}
			aggregatedAccounts[mint] = current
		} else {
			// New mint encountered, create the base TokenAccount
			acc := TokenAccount{
				Mint:         mint,
				Amount:       uiAmount,
				Decimals:     int(decimals),
				TokenAddress: tokenAcctAddr,
			}
			aggregatedAccounts[mint] = acc
		}
	}

	result := make([]TokenAccount, 0, len(aggregatedAccounts))

	// Concurrently fetch metadata
	var wg sync.WaitGroup
	resultsChan := make(chan TokenAccount, len(aggregatedAccounts))
	// Keep track of errors in a separate channel or slice if detailed error handling per token is needed
	// For simplicity here, we'll log errors within the goroutine and proceed.

	// Define a semaphore to limit concurrency for active operations
	const maxConcurrentMetadataFetches = 5 // Reduced concurrency
	semaphore := make(chan struct{}, maxConcurrentMetadataFetches)

	// Rate limiter: allows operations to start at a fixed interval
	// 4 requests per second = 1 request every 250ms
	// This ticker controls the rate at which goroutines can proceed to make an RPC call.
	rateLimitTicker := time.NewTicker(400 * time.Millisecond) // Changed from 300ms to 400ms
	defer rateLimitTicker.Stop()                              // Ensure ticker is stopped when function exits

	for mint, acc := range aggregatedAccounts {
		wg.Add(1)
		go func(m string, a TokenAccount) {
			defer wg.Done()

			// --- BEGIN PRE-CACHE CHECK ---
			// Attempt to load from cache directly first to avoid rate limiter for cache hits.
			cachedInfo, cacheErr := c.loadFullTokenInfoFromCache(m)

			if cacheErr == nil && cachedInfo != nil { // Cache hit and successfully loaded
				a.Name = cachedInfo.ResolvedName
				if a.Name == "" {
					a.Name = cachedInfo.OnChainName
				}
				a.Symbol = cachedInfo.ResolvedSymbol
				if a.Symbol == "" {
					a.Symbol = cachedInfo.OnChainSymbol
				}
				a.LogoURI = cachedInfo.ResolvedImageURI
				a.Description = cachedInfo.ResolvedDescription
				a.Attributes = cachedInfo.Attributes
				a.Extensions = cachedInfo.Extensions
				resultsChan <- a
				return // Successfully processed from cache, skip network fetch and rate limiting
			} else if cacheErr != nil { // An actual error occurred trying to read/parse the cache
				logger.ToolWarn("Attempt to load mint %s from disk cache (pre-check) failed: %v. Will proceed to network fetch.", m, cacheErr)
			}
			// If cachedInfo is nil (either due to clean miss (cacheErr == nil) or an error (cacheErr != nil)), we fall through.
			// Logging for clean miss is handled by loadFullTokenInfoFromCache.
			// --- END PRE-CACHE CHECK ---

			// Wait for the rate limiter's tick ONLY IF we are going to network.
			// This ensures we don't initiate RPC calls too rapidly.
			<-rateLimitTicker.C

			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			var finalFullInfo *FullTokenInfo
			var lastErr error

			const maxRetries = 3
			initialBackoff := 500 * time.Millisecond

			for attempt := 0; attempt <= maxRetries; attempt++ {
				metadataCtx, cancelAttempt := context.WithTimeout(ctx, 15*time.Second)
				currentAttemptFullInfo, currentAttemptErr := c.GetTokenMetadata(metadataCtx, m)
				cancelAttempt()

				lastErr = currentAttemptErr // Store the last error encountered

				if currentAttemptErr == nil {
					finalFullInfo = currentAttemptFullInfo // Success
					break                                  // Exit retry loop
				}

				// Manually replicate IsRPCErrorWithCode logic:
				var rpcErrInstance *jrpc.RPCError
				isRateLimitError := false
				if errors.As(currentAttemptErr, &rpcErrInstance) && rpcErrInstance != nil {
					if rpcErrInstance.Code == 429 {
						isRateLimitError = true
					}
				}

				if isRateLimitError {
					if attempt < maxRetries {
						waitDuration := initialBackoff * time.Duration(1<<attempt) // Exponential backoff
						logger.ToolWarn("Rate limit hit for mint %s (Code 429). Retrying in %v (attempt %d/%d). Error: %v", m, waitDuration, attempt+1, maxRetries, currentAttemptErr)

						var retrying = true
						select {
						case <-time.After(waitDuration):
							// Continue to next attempt
						case <-ctx.Done():
							logger.ToolWarn("Context cancelled during backoff for mint %s. Aborting retries. Last error: %v", m, lastErr)
							retrying = false // Signal to break from the outer loop
						}
						if !retrying {
							break // Break from the attempt loop if context was cancelled
						}
						continue // Next attempt
					} else {
						// Max retries for 429 reached
						logger.ToolWarn("Max retries reached for mint %s after rate limit errors (Code 429). Last error: %v", m, currentAttemptErr)
						break // Give up on retries, exit retry loop
					}
				} else {
					// Non-429 error, or not an RPC error we could parse
					logger.ToolWarn("Failed to get full metadata for mint %s in GetTokenBalances (non-429 or non-RPC error, attempt %d/%d): %v. Proceeding with partial info.", m, attempt+1, maxRetries, currentAttemptErr)
					break // Exit retry loop, do not retry this error
				}
			}

			if finalFullInfo != nil {
				// Use resolved name/symbol if available, otherwise fall back to on-chain, then empty.
				a.Name = finalFullInfo.ResolvedName
				if a.Name == "" {
					a.Name = finalFullInfo.OnChainName
				}
				a.Symbol = finalFullInfo.ResolvedSymbol
				if a.Symbol == "" {
					a.Symbol = finalFullInfo.OnChainSymbol
				}
				a.LogoURI = finalFullInfo.ResolvedImageURI
				a.Description = finalFullInfo.ResolvedDescription
				a.Attributes = finalFullInfo.Attributes
				a.Extensions = finalFullInfo.Extensions
			}
			// If finalFullInfo is nil (all attempts failed or non-retryable error), 'a' will be sent with only basic info.
			// The existing logging handles the case where lastErr is not nil and finalFullInfo is nil adequately.
			resultsChan <- a
		}(mint, acc)
	}

	go func() {
		wg.Wait()
		close(resultsChan)
	}()

	for enrichedAcc := range resultsChan {
		result = append(result, enrichedAcc)
	}

	return result, nil
}

// --------------- Solana Name Service (.sol resolution) ---------------

const (
	// NameServiceProgramIDString is the address of the SPL Name Service program.
	NameServiceProgramIDString = "namesLPneVptA9Z5rqUDD9tMTWEJwofgaYwp8cawRkX"
	// SolTLDAuthorityString is the Bonfida TLD authority for .sol domains.
	SolTLDAuthorityString = "58PwtjSDuFHuUkYjH9BYnnQKHfwo9reZhC2zMJv9JPkx"
)

var snsProgramID = solana.MustPublicKeyFromBase58(NameServiceProgramIDString)
var solTLDAuthority = solana.MustPublicKeyFromBase58(SolTLDAuthorityString)

// NameRecordHeaderLayout represents the structure of the .sol name record data header
// See: https://github.com/Bonfida/solana-name-service-guide/blob/master/registers/index.md
// And: https://github.com/Bonfida/solana-name-service/blob/master/program/src/state.rs
type NameRecordHeader struct {
	ParentName solana.PublicKey // 32 bytes - The parent name of the record. For .sol, this is solTLDAuthority
	Owner      solana.PublicKey // 32 bytes - The owner of the domain.
	Class      solana.PublicKey // 32 bytes - For .sol domains, this is usually set to the SystemProgramID or zeros.
	// data         []byte         // Variable length, starts after the 96-byte header. For SOL records, it's the pubkey.
}

// ResolveAddress takes a Solana address string or a .sol domain name and returns
// the base58 encoded public key.
// If addrOrName is already a valid base58 pubkey string, it's returned directly.
// Otherwise, it attempts to resolve it as a .sol name.
func (c *Client) ResolveAddress(ctx context.Context, addrOrName string) (string, error) {
	if _, err := solana.PublicKeyFromBase58(addrOrName); err == nil {
		return addrOrName, nil // It's already a valid pubkey
	}

	if !strings.HasSuffix(strings.ToLower(addrOrName), ".sol") {
		return "", fmt.Errorf("invalid address or .sol name: %s", addrOrName)
	}

	// Proceed with .sol name resolution
	domainName := strings.TrimSuffix(strings.ToLower(addrOrName), ".sol")
	hashedName := deriveHashedName(domainName)

	// Construct the name account PDA
	// Seeds: HashedName, NameClass (zero pubkey), ParentName (solTLDAuthority)
	nameAccountKey, _, err := solana.FindProgramAddress(
		[][]byte{
			hashedName,
			(solana.PublicKey{}).Bytes(), // Zero pubkey for NameClass
			solTLDAuthority.Bytes(),      // ParentName
		},
		snsProgramID,
	)
	if err != nil {
		return "", fmt.Errorf("failed to derive name account PDA for %s: %w", addrOrName, err)
	}

	// Fetch account data
	accInfo, err := c.rpcClient.GetAccountInfo(ctx, nameAccountKey)
	if err != nil {
		return "", fmt.Errorf("failed to get account info for %s (derived from %s): %w", nameAccountKey, addrOrName, err)
	}
	if accInfo == nil || accInfo.Value == nil {
		return "", fmt.Errorf("name account %s not found for .sol name %s", nameAccountKey, addrOrName)
	}
	if accInfo.Value.Owner != snsProgramID {
		return "", fmt.Errorf("name account %s owner is not SNS program for .sol name %s", nameAccountKey, addrOrName)
	}

	accountData := accInfo.Value.Data.GetBinary()
	if len(accountData) < 96 { // Minimum length for NameRecordHeader
		return "", fmt.Errorf("name record data for %s is too short: got %d bytes, expected at least 96", addrOrName, len(accountData))
	}

	// Deserialize the header. Using binary.NewDecoder for potentially complex struct later if needed.
	// For now, we only care about the owner field from the header which is at offset 32.
	// The actual "data" part of the record (SOL record data for .sol names) starts *after* the 96-byte header.
	// However, for .sol names, the owner of the *name record itself* (NameRecordHeader.Owner) is the resolved address.

	// var header NameRecordHeader
	// We need to decode the NameRecordHeader from the *beginning* of the accountData
	// The owner of the domain is the second Pubkey in the NameRecordHeader struct (offset 32 bytes)
	if len(accountData) < (32 + 32) { // ParentName + Owner
		return "", fmt.Errorf("name record data for %s is too short to extract owner: got %d bytes", addrOrName, len(accountData))
	}

	ownerBytes := accountData[32 : 32+32]
	resolvedOwnerPk := solana.PublicKeyFromBytes(ownerBytes)

	return resolvedOwnerPk.String(), nil
}

func deriveHashedName(name string) []byte {
	// SHA256 hash of ("\x01" + name) where \x01 is the NameClass prefix for .sol TLD
	// This seems to be a common way, but Bonfida's own JS code might have nuances.
	// The SNS program itself expects the HASH_PREFIX + Name, where HASH_PREFIX is "SPL Name Service"
	// For child domains, it's different. For .sol TLD directly, it's simpler.
	// From Bonfida's JS: `keccak_256(Buffer.from(ROOT_DOMAIN_ACCOUNT_KEY.toString() + domainTld))` - this is for deriving the *parent* for subdomains.
	// For the actual name account: `keccak256(Buffer.concat([Buffer.from(HASH_PREFIX), Buffer.from(name)]))`
	// HASH_PREFIX = 'SPL Name Service'

	// The crucial part for deriving the PDA for a name like "bonfida.sol" involves:
	// 1. Hashed name: SHA256 of (HASH_PREFIX + "bonfida") -> HashedName
	// 2. NameClass: For .sol, this is effectively Zero Pubkey
	// 3. ParentName: This is solTLDAuthority = 58Pwtj...

	// For "bonfida.sol", the `name` part is "bonfida".
	hashPrefix := "SPL Name Service"
	input := hashPrefix + name
	hasher := sha256.New()
	hasher.Write([]byte(input))
	return hasher.Sum(nil)
}
