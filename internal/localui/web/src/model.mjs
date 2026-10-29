export const initial = {
  project: '.',
  platform: 'ios',
  payments: 'none',
  accounts: false,
  tracking: false,
  localized: false,
  locales: [],
  screenshots: false,
  screenshot_path: '',
  display_type: 'APP_IPHONE_67',
  language: 'eng',
  products: [],
  source_files: [],
  storekit_file: '',
  asc_products_file: '',
  revenuecat_file: '',
  live_asc: false,
  app_id: '',
  version_id: '',
  build: '',
};

export const labels = {
  source: 'Source & privacy',
  screenshots: 'Screenshots',
  purchases: 'Purchase configuration',
  localization: 'Localization coverage',
  accounts: 'Account flows',
  tracking: 'Tracking consent',
  asc: 'App Store Connect',
};

export function counts(rows = []) {
  return rows.reduce(
    (out, row) => {
      out[row.status] = (out[row.status] || 0) + 1;
      return out;
    },
    { failed: 0, suspected: 0, passed: 0, unverified: 0 },
  );
}

export function validate(s) {
  if (s.screenshots && !s.screenshot_path.trim()) return 'Choose a screenshot folder or file.';
  if (s.payments !== 'none' && (!s.products.length || s.products.some((p) => !p.id.trim())))
    return 'Add an expected product ID for each purchase row.';
  if (s.payments === 'revenuecat' && s.products.some((p) => !p.entitlement?.trim()))
    return 'Add the expected entitlement for each RevenueCat product.';
  if (s.live_asc && (!s.app_id.trim() || !s.version_id.trim()))
    return 'Enter the app ID and version resource ID for App Store Connect.';
  return '';
}

export function verdict(summary) {
  if (!summary) return 'Audit in progress';
  if (summary.failed || summary.suspected) return 'A few things to review.';
  if (summary.incomplete) return 'Some evidence is missing.';
  return 'No blockers found.';
}

// Persist only the session credential in this tab; app inputs and reports are
// intentionally not written to browser storage.
export function sessionToken(location, storage) {
  const incoming = location.hash.slice(1);
  if (/^[a-f0-9]{64}$/.test(incoming)) {
    storage.setItem('greenlight-session', incoming);
    return incoming;
  }
  return storage.getItem('greenlight-session') || '';
}

export function checkTitle(row) {
  return (
    {
      'source-patterns': 'Source patterns',
      'app-metadata': 'App metadata',
      dimensions: 'Screenshot dimensions',
      'alpha-channel': 'Image transparency',
      'pricing-copy': 'Pricing & trial copy',
      'placeholder-copy': 'Placeholder copy',
      'platform-reference': 'Other platform references',
      'source-product-id': 'Product IDs in source',
      'storekit-product-id': 'StoreKit product IDs',
      'app-store-connect-product-id': 'App Store product IDs',
      'asc-product-state': 'App Store product status',
      'revenuecat-product-id': 'RevenueCat product IDs',
      'revenuecat-entitlement': 'RevenueCat entitlement mapping',
      'sandbox-purchase': 'Sandbox purchase',
      'restore-purchases': 'Restore purchases',
      'refund-revocation': 'Refund & revocation',
      'submission-product-attachment': 'Products attached to submission',
      'locale-screenshots': 'Screenshot locale coverage',
    }[row.id] || row.title
  );
}
