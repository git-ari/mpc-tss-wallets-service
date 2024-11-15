BASE_URL=http://localhost:8080

# Retrieve all wallets
get-wallets:
	curl -X GET "$(BASE_URL)/wallets" -H "Accept: application/json"

# Generate a new wallet
create-wallet:
	curl -X POST "$(BASE_URL)/wallet" -H "Accept: application/json"

# Sign data (transactions) with a wallet
sign-data:
	curl -X POST "$(BASE_URL)/sign" -d '{"data": "$(data)", "wallet": "$(wallet)"}' \
		 -H "Accept: application/json" -H "Content-Type: application/json"

# Runs a full example of the service functionalities
full-example:
	@echo "Creating new wallet..."
	@address=$$(curl -s -X POST "${BASE_URL}/wallet" -H "Accept: application/json" | jq -r '.address'); \
	echo "Successfully created new wallet with address: $$address"; \
	\
	echo "\nSigning data with new wallet..."; \
	echo "Example Data is \"0x74657374\" (aka \"test\")"; \
	$(MAKE) --no-print-directory sign-data data="0x74657374" wallet=$$address; \
	\
	echo "\nSuccessfully signed data"; \
	echo "\nListing wallets..."; \
	$(MAKE) --no-print-directory get-wallets
	
help:
	@echo "Usage:"
	@echo "make get-wallets"
	@echo "make create-wallet"
	@echo "make sign-data data=\"example_data\" wallet=\"example_wallet_address\""
