# Webhook Echo

Dead stupid simple echo server which records incoming webhooks in a sqlite (in memory) database. Provides a simple API interface to query recorded webhooks.

I'm using this as a sink for integration tests in a webapp I'm developing.

## Usage

This repo contains a flake.nix, `nix develop`, `nix build` and `nix run` should all work.
