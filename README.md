# MPC based TSS Wallets Service

This is a Go application implementing a MPC based Threshold Signature Scheme (TSS) wallet service using a TSS library. The service allows clients to create wallets and sign data (e.g. transactions) in a distributed manner without revealing the private key to any single party.

It was built using the [tss-lib](https://github.com/bnb-chain/tss-lib), which simplifies the process of distributed key generation and signing. [Gin Gonic](https://github.com/gin-gonic/gin) was used for handling API endpoints due to its ease of use and performance. While the `tss-lib` requires the use of channels for concurrency, additional channels were introduced to improve performance and scalability, especially as the number of parties per wallet increases.


## Prerequisites

- **Go** Version 1.23.3

## Installation

1. Clone the Repository

```bash
git clone https://github.com/git-ari/mpc-tss-wallets-service.git
cd mpc-tss-wallets-service
```

2. Install Dependencies
```bash
go mod download
```

## Run the service

```bash
go run main.go
```


## Makefile Commands

A **Makefile** is provided to simplify testing and interaction with the API endpoints.

### Available Commands

- **get-wallets**: Retrieve all wallets.

    ```bash
    make get-wallets
    ```

- **create-wallet**: Generate a new wallet.

    ```bash
    make create-wallet
    ```

- sign-data: Sign data with a wallet.

    ```bash
    make sign-data data="0x74657374" wallet="0xYourWalletAddress"
    ```

- **full-example**: Runs a full example of the service functionalities.

    ```bash
    make full-example

    ```

- **help**: Display usage information.

    ```bash
    make help
    ```
## Running tests

In order to run the tests, use the command:

```bash
go test
```

## Reference

- https://mmasmoudi.medium.com/an-overview-of-multi-party-computation-mpc-threshold-signatures-tss-and-mpc-tss-wallets-4253adacd1b2