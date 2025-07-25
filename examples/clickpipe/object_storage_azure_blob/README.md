## ClickPipe Object Storage with Azure Blob Storage example

This example demonstrates how to deploy a ClickPipe with an Azure Blob Storage container as the input source,
authenticated with a connection string.

## Prerequisites

- Azure Storage Account with a blob container
- Files in the container (e.g., JSON files in `data/` folder)
- Azure Storage Account connection string

## How to run

- Rename `variables.sample.tfvars` to `variables.tfvars` and fill in all needed data.
- Run `terraform init`
- Run `terraform <plan|apply> -var-file=variables.tfvars`

## Azure Blob Storage Configuration

The example uses:
- `type = "azureblobstorage"` for Azure Blob Storage
- `authentication = "CONNECTION_STRING"` with Azure connection string
- `azure_container_name` to specify the container
- `path` to specify file path within the container (supports wildcards)

## Connection String Format

The Azure connection string should be in the format:
```
DefaultEndpointsProtocol=https;AccountName=<account_name>;AccountKey=<account_key>;EndpointSuffix=core.windows.net
```
