# api

## 2.7.0 (2026-10-05)

Batched ledger writes

The API accepts ledger writes in batches of up to 500 entries. Migration 0007 adds the batches table and applies before the new code.

### Added

- Batched ledger writes

# es-runtime

## 1.2.0 (2026-10-10)

Streaming results and safer retries

Results stream from tern as it produces them, and the runtime no longer retries after a non-transient exit.

### Added

- Stream results as tern produces them

### Fixed

- No retries after a non-transient exit

# go-sdk

## 0.9.0 (2026-10-07)

Retries with backoff in ledgerclient

ledgerclient retries idempotent calls with exponential backoff, so transient API errors no longer surface to callers.

### Fixed

- Idempotent calls retry with backoff

# notify

## 1.0.0 (2026-10-05)

Daily digest emails

The first notify release sends each workspace a daily digest of ledger activity.

### Added

- Daily digest emails

# site-docs

## 1.5.0 (2026-10-07)

New pricing page

The docs site adds a pricing page.

### Documentation

- Pricing page

# tern

## 0.4.0-rc.2 (2026-10-18)

_Prerelease on the rc channel._

CSV escaping fix on top of rc.1

Second release candidate for 0.4.0. It carries everything in rc.1 and fixes quoting of commas and newlines in CSV report output.

### Added

- Install tern from npm with @tern/cli

### Fixed

- CSV reports quote commas and newlines
- Ledger totals round half-even

## 0.3.0 (2026-10-02)

PDF reports and multi-currency ledgers

tern reports can be exported as PDF, and ledgers hold amounts in several currencies, converted through embedded rate tables.

### Added

- Export reports as PDF
- Ledgers hold amounts in several currencies

### Migration

- Existing ledgers default to the currency in tern.toml.
