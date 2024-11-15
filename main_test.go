package main

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestCreateWallet(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.Default()
	router.POST("/wallet", createWallet)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/wallet", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]string
	err := json.Unmarshal(w.Body.Bytes(), &response)
	if err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	address, exists := response["address"]
	assert.True(t, exists, "Response should contain 'address' key")
	assert.NotEmpty(t, address, "Address should not be empty")
}

func TestListWallets(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.Default()
	router.POST("/wallet", createWallet)
	router.GET("/wallets", listWallets)

	// Create a wallet to ensure the list is not empty
	w1 := httptest.NewRecorder()
	req1, _ := http.NewRequest("POST", "/wallet", nil)
	router.ServeHTTP(w1, req1)
	assert.Equal(t, http.StatusOK, w1.Code)

	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("GET", "/wallets", nil)
	router.ServeHTTP(w2, req2)

	assert.Equal(t, http.StatusOK, w2.Code)

	var response map[string][]walletsResponse
	err := json.Unmarshal(w2.Body.Bytes(), &response)
	if err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	walletsResp, exists := response["wallets"]
	assert.True(t, exists, "Response should contain 'wallets' key")
	assert.NotEmpty(t, walletsResp, "Wallets list should not be empty")
	assert.Len(t, walletsResp, 1, "Wallets list should contain only one wallet")
	assert.True(t, strings.HasPrefix(walletsResp[0].Address, "0x"), "Address should start with '0x'")
	address, err := hex.DecodeString(walletsResp[0].Address[2:])
	assert.NoError(t, err, "Address should be a valid hex string")
	assert.Len(t, address, 20, "Address should be 20 bytes long")

	assert.NotEmpty(t, walletsResp[0].PubKey, "Wallet public key should not be empty")
	assert.True(t, strings.HasPrefix(walletsResp[0].PubKey, "0x"), "Public key should start with '0x'")
	pubKey, err := hex.DecodeString(walletsResp[0].PubKey[2:])
	assert.NoError(t, err, "Public key should be a valid hex string")
	assert.Len(t, pubKey, 64, "Public key should be 64 bytes long")
}

func TestSignData(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.Default()
	router.POST("/wallet", createWallet)
	router.POST("/sign", signData)

	// Create a wallet
	w1 := httptest.NewRecorder()
	req1, _ := http.NewRequest("POST", "/wallet", nil)
	router.ServeHTTP(w1, req1)
	assert.Equal(t, http.StatusOK, w1.Code)

	// Get the wallet address
	var createResponse map[string]string
	err := json.Unmarshal(w1.Body.Bytes(), &createResponse)
	if err != nil {
		t.Fatalf("Failed to parse create wallet response: %v", err)
	}
	walletAddress, exists := createResponse["address"]
	assert.True(t, exists)
	assert.NotEmpty(t, walletAddress)

	requestBody := signDataRequest{
		Data:   "0x74657374", // "test" in hex
		Wallet: walletAddress,
	}
	jsonBody, _ := json.Marshal(requestBody)

	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("POST", "/sign", bytes.NewBuffer(jsonBody))
	req2.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w2, req2)

	assert.Equal(t, http.StatusOK, w2.Code)

	var response map[string]string
	err = json.Unmarshal(w2.Body.Bytes(), &response)
	if err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	signature, exists := response["signature"]
	assert.True(t, exists, "Response should contain 'signature' key")
	assert.NotEmpty(t, signature, "Signature should not be empty")
	_, err = hex.DecodeString(signature)
	assert.NoError(t, err, "Signature should be a valid hex string")
}

func TestSignDataInvalidInput(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.Default()
	router.POST("/sign", signData)

	// Data and wallet are required hence it will fail
	requestBody := signDataRequest{
		Data:   "",
		Wallet: "",
	}
	jsonBody, _ := json.Marshal(requestBody)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/sign", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestSignDataNonExistentWallet(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.Default()
	router.POST("/sign", signData)

	requestBody := signDataRequest{
		Data:   "0x74657374", // "test" in hex
		Wallet: "0xadcdf1cc67362d0d61ad8954d077b78a1d80087b",
	}
	jsonBody, _ := json.Marshal(requestBody)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/sign", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestSignDataInvalidData(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.Default()
	router.POST("/sign", signData)

	requestBody := signDataRequest{
		Data:   "0x7$41657374",
		Wallet: "0xNonExistentWallet",
	}
	jsonBody, _ := json.Marshal(requestBody)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/sign", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestIntegrationWorkflow(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.Default()
	router.POST("/wallet", createWallet)
	router.GET("/wallets", listWallets)
	router.POST("/sign", signData)

	// Create a Wallet
	w1 := httptest.NewRecorder()
	req1, _ := http.NewRequest("POST", "/wallet", nil)
	router.ServeHTTP(w1, req1)
	assert.Equal(t, http.StatusOK, w1.Code)

	var createResponse map[string]string
	err := json.Unmarshal(w1.Body.Bytes(), &createResponse)
	if err != nil {
		t.Fatalf("Failed to parse create wallet response: %v", err)
	}
	walletAddress, exists := createResponse["address"]
	assert.True(t, exists)
	assert.NotEmpty(t, walletAddress)

	// List Wallets
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("GET", "/wallets", nil)
	router.ServeHTTP(w2, req2)
	assert.Equal(t, http.StatusOK, w2.Code)

	var listResponse map[string][]walletsResponse
	err = json.Unmarshal(w2.Body.Bytes(), &listResponse)
	if err != nil {
		t.Fatalf("Failed to parse list wallets response: %v", err)
	}
	walletsList, exists := listResponse["wallets"]
	assert.True(t, exists)
	assert.NotEmpty(t, walletsList)

	// Sign Data
	requestBody := signDataRequest{
		Data:   "0x74657374", // "test" in hex
		Wallet: walletAddress,
	}
	jsonBody, _ := json.Marshal(requestBody)

	w3 := httptest.NewRecorder()
	req3, _ := http.NewRequest("POST", "/sign", bytes.NewBuffer(jsonBody))
	req3.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w3, req3)
	assert.Equal(t, http.StatusOK, w3.Code)

	var signResponse map[string]string
	err = json.Unmarshal(w3.Body.Bytes(), &signResponse)
	if err != nil {
		t.Fatalf("Failed to parse sign data response: %v", err)
	}
	signature, exists := signResponse["signature"]
	assert.True(t, exists)
	assert.NotEmpty(t, signature)
}
