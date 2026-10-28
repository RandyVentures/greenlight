# Purchase catalog comparison

```sh
greenlight purchases examples/purchases/config.json --format json --exit-code
```

All paths resolve relative to the configuration file. No API keys are used,
no account actions occur, and no transactions are initiated.

## Configuration

```json
{
  "products": [{"id": "com.example.pro", "entitlement": "pro"}],
  "source_files": ["Products.swift"],
  "storekit_file": "Local.storekit",
  "app_store_connect_file": "asc-products.json",
  "revenuecat_file": "revenuecat-products.json"
}
```

List expected products for this build. Extra catalog products are allowed to
preserve old subscriptions. Configuration typos, duplicate expected IDs, and
duplicate catalog products are rejected or marked unverified.

Source files must contain an exact quoted ID (single, double, or backtick quotes).
This is a literal search, not a compiler or data-flow analysis: dead code and
comments can contain matching literals. IDs fetched from a server or assembled
at runtime need manual confirmation. Missing or unreadable source evidence cannot
produce a clean result.

## StoreKit

Supply an Xcode `.storekit` JSON file. `products[].productID` and
`subscriptionGroups[].subscriptions[].productID` are read. Omitting the file is
optional/unverified because a local StoreKit file is not required for submission.
If supplied, it must contain each configured expected product.

## App Store Connect snapshot

Supply an ASC API-style `data` array of products and/or subscriptions:

```json
{
  "data": [
    {"attributes": {"productId": "com.example.pro", "state": "APPROVED"}}
  ]
}
```

Both `state` and `inAppPurchaseState` are supported. Merge all relevant IAP and
subscription pages for the intended app before comparing. A nonempty
`links.next` makes the snapshot unverified. This tool cannot prove that an export
with `links.next` removed was complete; completeness is the caller's responsibility.

`APPROVED` satisfies the product-state check. `READY_TO_SUBMIT` and review-pending
states warn to confirm inclusion in the intended submission. Missing, unrecognized,
or unreadable states are unverified. Rejected, missing-metadata, and removed-from-sale
products produce actionable findings. An approved product alone does not prove
availability, correct pricing, agreements, or a working purchase.

## RevenueCat snapshot (normalized format)

This explicit format is **not a raw RevenueCat API response** or subscriber export.
Create it from the product/entitlement configuration for the intended app:

```json
{
  "schema_version": 1,
  "products": [
    {"id": "com.example.pro", "entitlements": ["pro"]}
  ]
}
```

Use the store product identifier for `id`, not RevenueCat's internal resource ID.
Use entitlement identifiers for `entitlements`. A RevenueCat snapshot is required
when expected products declare an `entitlement`. Apps using StoreKit directly can
omit both. No automatic RevenueCat fetching or offering/package validation is
implemented.

## What remains unverified

Sandbox purchase, restore, refund/revocation, and submission-product attachment
remain unverified. A catalog match cannot validate transactions or grant/restore
entitlements. Snapshots can be stale or copied from the wrong app/environment.
Verify inputs and test the candidate build on an appropriate sandbox account
before submission.
