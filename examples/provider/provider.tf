terraform {
  required_providers {
    capydb = {
      source  = "capy-base/capydb"
      version = "~> 0.1"
    }
  }
}

provider "capydb" {
  # Falls back to the CAPYDB_API_KEY environment variable when omitted.
  api_key = var.capydb_api_key

  # Optional. Falls back to CAPYDB_API_URL, then to https://capydb.dev/api/capydb.
  # api_url = "https://capydb.dev/api/capydb"
}

variable "capydb_api_key" {
  type      = string
  sensitive = true
}
