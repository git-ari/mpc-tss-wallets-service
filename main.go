package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"encoding/hex"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"sync"

	"github.com/bnb-chain/tss-lib/common"
	tsscrypto "github.com/bnb-chain/tss-lib/crypto"
	"github.com/bnb-chain/tss-lib/ecdsa/keygen"
	"github.com/bnb-chain/tss-lib/ecdsa/signing"
	"github.com/bnb-chain/tss-lib/tss"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/gin-gonic/gin"
)

// signDataRequest represents the request body for signData endpoint
type signDataRequest struct {
	Data   string `json:"data"`
	Wallet string `json:"wallet"`
}

// walletsResponse represents the response body for list wallets endpoint
type walletsResponse struct {
	Address string `json:"address"`
	PubKey  string `json:"pubKey"`
}

// Wallet represents a TSS wallet with its associated data
type Wallet struct {
	Address   string
	PartyIDs  tss.SortedPartyIDs
	Threshold int
	PubKey    *ecdsa.PublicKey
	SaveData  map[string]*keygen.LocalPartySaveData
}

// keygenResult holds the result of the key generation for a party
type keygenResult struct {
	PartyID *tss.PartyID
	Save    keygen.LocalPartySaveData
}

// Global variables to store wallets and synchronize access
var (
	wallets      = make(map[string]*Wallet)
	walletsMutex sync.Mutex
)

func main() {
	r := gin.Default()
	r.POST("/wallet", createWallet)
	r.GET("/wallets", listWallets)
	r.POST("/sign", signData)
	r.Run(":8080")
}

// createWallet handles the creation of a new TSS wallet
func createWallet(c *gin.Context) {
	parties := 3
	threshold := 1

	// Lock wallets map to get the current count and avoid race conditions
	walletsMutex.Lock()
	existingWallets := len(wallets)
	walletsMutex.Unlock()

	// Generate unique party IDs
	partyIDs := make([]*tss.PartyID, parties)
	key := common.MustGetRandomInt(256)
	for i := 0; i < parties; i++ {
		id := fmt.Sprintf("%d", existingWallets+i)
		moniker := fmt.Sprintf("P[%d]", existingWallets+i)
		// Ensure unique key for each party
		keyShare := new(big.Int).Sub(key, big.NewInt(int64(existingWallets)-int64(i)))
		partyIDs[i] = tss.NewPartyID(id, moniker, keyShare)
	}
	partyIDs = tss.SortPartyIDs(partyIDs)
	ctx := tss.NewPeerContext(partyIDs)

	// Channels for communication
	errCh := make(chan *tss.Error)
	outChs := make([]chan tss.Message, parties)
	endChs := make([]chan keygen.LocalPartySaveData, parties)
	resultCh := make(chan keygenResult, parties)
	messages := make(chan tss.Message, parties*parties)

	// Start key generation parties
	partiesList := make([]*keygen.LocalParty, parties)
	for i, partyID := range partyIDs {
		params := tss.NewParameters(tss.S256(), ctx, partyID, parties, threshold)
		outCh := make(chan tss.Message, parties*parties)
		endCh := make(chan keygen.LocalPartySaveData, 1)
		outChs[i] = outCh
		endChs[i] = endCh
		party := keygen.NewLocalParty(params, outCh, endCh).(*keygen.LocalParty)
		partiesList[i] = party

		// Start each party in a separate goroutine
		go func(p *keygen.LocalParty, partyID *tss.PartyID) {
			if err := p.Start(); err != nil {
				errCh <- err
				return
			}
			save := <-endCh
			resultCh <- keygenResult{PartyID: partyID, Save: save}
		}(party, partyID)
	}

	// Forward messages from parties to the messages channel
	for _, outCh := range outChs {
		go func(ch chan tss.Message) {
			for msg := range ch {
				messages <- msg
			}
		}(outCh)
	}

	// Handle message passing and collect results
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		saves := make(map[string]*keygen.LocalPartySaveData)
		var pubKey *tsscrypto.ECPoint
		for {
			select {
			case err := <-errCh:
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			case msg := <-messages:
				wireBytes, _, err := msg.WireBytes()
				if err != nil {
					errCh <- tss.NewError(err, "failed to serialize wire bytes", 0, msg.GetFrom(), nil)
					return
				}
				dest := msg.GetTo()
				if dest == nil { // Broadcast message
					for _, p := range partiesList {
						if p.PartyID().Id == msg.GetFrom().Id {
							continue
						}
						go func(p *keygen.LocalParty) {
							if _, err := p.UpdateFromBytes(wireBytes, msg.GetFrom(), msg.IsBroadcast()); err != nil {
								errCh <- err
							}
						}(p)
					}
				} else { // Point-to-point message
					for _, to := range dest {
						for _, p := range partiesList {
							if p.PartyID().Id == to.Id {
								go func(p *keygen.LocalParty) {
									if _, err := p.UpdateFromBytes(wireBytes, msg.GetFrom(), msg.IsBroadcast()); err != nil {
										errCh <- err
									}
								}(p)
								break
							}
						}
					}
				}
			case result := <-resultCh:
				partyIDStr := result.PartyID.Id
				saves[partyIDStr] = &result.Save
				if pubKey == nil {
					pubKey = result.Save.ECDSAPub
				}
				if len(saves) == parties {
					// All parties have completed keygen
					x, y := pubKey.X(), pubKey.Y()
					pubKeyECDSA := ecdsa.PublicKey{
						Curve: elliptic.P256(),
						X:     x,
						Y:     y,
					}
					address := crypto.PubkeyToAddress(pubKeyECDSA).Hex()

					wallet := &Wallet{
						Address:   address,
						PubKey:    &pubKeyECDSA,
						SaveData:  saves,
						PartyIDs:  partyIDs,
						Threshold: threshold,
					}
					walletsMutex.Lock()
					wallets[address] = wallet
					walletsMutex.Unlock()
					c.JSON(http.StatusOK, gin.H{"address": address})
					return
				}
			}
		}
	}()
	wg.Wait()
}

// listWallets returns a list of all created wallets
func listWallets(c *gin.Context) {
	walletsMutex.Lock()
	defer walletsMutex.Unlock()

	walletsResp := make([]walletsResponse, 0, len(wallets))
	for addr, wallet := range wallets {
		walletsResp = append(walletsResp, walletsResponse{
			Address: addr,
			// Removing the first byte as it is not necesary since its a prefix
			PubKey: fmt.Sprintf("0x%x", crypto.FromECDSAPub(wallet.PubKey)[1:]),
		})
	}
	c.JSON(http.StatusOK, gin.H{"wallets": walletsResp})
}

// signData handles the signing of data using a specified wallet
func signData(c *gin.Context) {
	var requestBody signDataRequest

	if err := c.BindJSON(&requestBody); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	dataHex := requestBody.Data
	walletAddress := requestBody.Wallet
	if dataHex == "" || walletAddress == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "data and wallet are required"})
		return
	}

	dataHex = strings.TrimPrefix(dataHex, "0x")
	data, err := hex.DecodeString(dataHex)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid data"})
		return
	}

	walletsMutex.Lock()
	wallet, exists := wallets[walletAddress]
	walletsMutex.Unlock()
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "wallet not found"})
		return
	}

	partyIDs := wallet.PartyIDs
	ctx := tss.NewPeerContext(partyIDs)

	// Convert data to *big.Int for signing
	msgToSign := new(big.Int).SetBytes(data)

	numParties := len(partyIDs)
	threshold := wallet.Threshold

	// Channels for communication
	errCh := make(chan *tss.Error)
	outChs := make([]chan tss.Message, numParties)
	endCh := make(chan common.SignatureData, numParties)
	messages := make(chan tss.Message, numParties*numParties)

	// Start signing parties.
	partiesList := make([]*signing.LocalParty, numParties)
	for i, partyID := range partyIDs {
		params := tss.NewParameters(tss.S256(), ctx, partyID, numParties, threshold)
		partyIDStr := partyID.Id
		saveData, exists := wallet.SaveData[partyIDStr]
		if !exists {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "SaveData for party not found"})
			return
		}
		outCh := make(chan tss.Message, numParties*numParties)
		outChs[i] = outCh
		party := signing.NewLocalParty(msgToSign, params, *saveData, outCh, endCh).(*signing.LocalParty)
		partiesList[i] = party

		// Start each party in a separate goroutine
		go func(p *signing.LocalParty) {
			if err := p.Start(); err != nil {
				errCh <- err
			}
		}(party)
	}

	// Forward messages from parties to the messages channel
	for _, outCh := range outChs {
		go func(ch chan tss.Message) {
			for msg := range ch {
				messages <- msg
			}
		}(outCh)
	}

	// Handle message passing and collect signatures
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		signatures := make([]*common.SignatureData, 0, numParties)
		for {
			select {
			case err := <-errCh:
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			case msg := <-messages:
				wireBytes, _, err := msg.WireBytes()
				if err != nil {
					errCh <- tss.NewError(err, "failed to serialize wire bytes", 0, msg.GetFrom(), nil)
					return
				}
				dest := msg.GetTo()
				if dest == nil { // Broadcast message
					for _, p := range partiesList {
						if p.PartyID().Id == msg.GetFrom().Id {
							continue
						}
						go func(p *signing.LocalParty) {
							if _, err := p.UpdateFromBytes(wireBytes, msg.GetFrom(), msg.IsBroadcast()); err != nil {
								errCh <- err
							}
						}(p)
					}
				} else { // Point-to-point message
					for _, to := range dest {
						for _, p := range partiesList {
							if p.PartyID().Id == to.Id {
								go func(p *signing.LocalParty) {
									if _, err := p.UpdateFromBytes(wireBytes, msg.GetFrom(), msg.IsBroadcast()); err != nil {
										errCh <- err
									}
								}(p)
								break
							}
						}
					}
				}
			case sigData := <-endCh:
				signatures = append(signatures, &sigData)
				if len(signatures) == numParties {
					// All parties have completed signing
					r, s := sigData.R, sigData.S
					signature := append(r, s...)
					c.JSON(http.StatusOK, gin.H{"signature": hex.EncodeToString(signature)})
					return
				}
			}
		}
	}()
	wg.Wait()
}
