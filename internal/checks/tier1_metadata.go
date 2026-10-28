package checks

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/RandyVentures/greenlight/internal/asc"
	"github.com/RandyVentures/greenlight/internal/screenshots"
)

// checkAppExists verifies the app is accessible via the API.
func checkAppExists(ctx context.Context, client *asc.Client, appID string, findings *[]Finding) error {
	app, err := client.GetApp(appID)
	if err != nil {
		return err
	}

	if app.Attributes.Name == "" {
		*findings = append(*findings, Finding{
			Tier:      TierMetadata,
			Severity:  SeverityBlock,
			Guideline: "2.3",
			Title:     "App name is empty",
			Detail:    "The app has no name set in App Store Connect.",
			Fix:       "Set the app name in App Store Connect → App Information.",
		})
	}

	return nil
}

// checkVersionPrepared verifies a version exists in a submittable state.
func checkVersionPrepared(ctx context.Context, client *asc.Client, appID string, findings *[]Finding) error {
	versions, err := client.GetAppStoreVersions(appID)
	if err != nil {
		return err
	}

	if len(versions) == 0 {
		*findings = append(*findings, Finding{
			Tier:     TierMetadata,
			Severity: SeverityBlock,
			Title:    "No version found",
			Detail:   "No App Store version in a submittable state.",
			Fix:      "Create a new version in App Store Connect.",
		})
		return nil
	}

	latest := versions[0]
	state := latest.Attributes.AppStoreState
	if state != "PREPARE_FOR_SUBMISSION" && state != "DEVELOPER_REJECTED" {
		*findings = append(*findings, Finding{
			Tier:     TierMetadata,
			Severity: SeverityInfo,
			Title:    fmt.Sprintf("Version %s is in state: %s", latest.Attributes.VersionString, state),
			Detail:   "Version may not be editable in its current state.",
		})
	}

	return nil
}

// App Store Connect metadata limits.
const (
	maxDescriptionLength     = 4000
	maxKeywordsLength        = 100
	maxWhatsNewLength        = 4000
	maxPromotionalTextLength = 170
	maxAppNameLength         = 30
	maxSubtitleLength        = 30
)

// checkMetadataCompleteness verifies all required metadata fields and their length limits.
func checkMetadataCompleteness(ctx context.Context, client *asc.Client, appID string, findings *[]Finding) error {
	versions, err := client.GetAppStoreVersions(appID)
	if err != nil || len(versions) == 0 {
		return err
	}

	localizations, err := client.GetVersionLocalizations(versions[0].ID)
	if err != nil {
		return err
	}

	if len(localizations) == 0 {
		*findings = append(*findings, Finding{
			Tier:      TierMetadata,
			Severity:  SeverityBlock,
			Guideline: "2.3",
			Title:     "No localizations found",
			Detail:    "Your app version has no localized metadata.",
			Fix:       "Add at least one localization with description, keywords, and screenshots.",
		})
		return nil
	}

	for _, loc := range localizations {
		attrs := loc.Attributes
		locale := attrs.Locale

		// Description: required + length limit
		desc := strings.TrimSpace(attrs.Description)
		if desc == "" {
			*findings = append(*findings, Finding{
				Tier:      TierMetadata,
				Severity:  SeverityBlock,
				Guideline: "2.3",
				Title:     fmt.Sprintf("[%s] Description is empty", locale),
				Detail:    "A description is required for App Store submission.",
				Fix:       "Add a description in App Store Connect → Version Information.",
			})
		} else if utf8.RuneCountInString(desc) > maxDescriptionLength {
			*findings = append(*findings, Finding{
				Tier:      TierMetadata,
				Severity:  SeverityBlock,
				Guideline: "2.3",
				Title:     fmt.Sprintf("[%s] Description exceeds %d character limit (%d chars)", locale, maxDescriptionLength, utf8.RuneCountInString(desc)),
				Detail:    "App Store Connect enforces a maximum description length.",
				Fix:       fmt.Sprintf("Shorten your description to %d characters or less.", maxDescriptionLength),
			})
		}

		// Keywords: recommended + length limit (100 chars including commas)
		kw := strings.TrimSpace(attrs.Keywords)
		if kw == "" {
			*findings = append(*findings, Finding{
				Tier:      TierMetadata,
				Severity:  SeverityWarn,
				Guideline: "2.3",
				Title:     fmt.Sprintf("[%s] Keywords are empty", locale),
				Detail:    "Keywords help users discover your app and are recommended.",
				Fix:       "Add relevant keywords separated by commas.",
			})
		} else if utf8.RuneCountInString(kw) > maxKeywordsLength {
			*findings = append(*findings, Finding{
				Tier:      TierMetadata,
				Severity:  SeverityBlock,
				Guideline: "2.3",
				Title:     fmt.Sprintf("[%s] Keywords exceed %d character limit (%d chars)", locale, maxKeywordsLength, utf8.RuneCountInString(kw)),
				Detail:    "Keywords field has a strict 100-character limit including commas and spaces.",
				Fix:       "Shorten your keywords to 100 characters. Remove less important terms or use shorter synonyms.",
			})
		}

		// What's New
		if strings.TrimSpace(attrs.WhatsNew) == "" {
			*findings = append(*findings, Finding{
				Tier:      TierMetadata,
				Severity:  SeverityWarn,
				Guideline: "2.3",
				Title:     fmt.Sprintf("[%s] 'What's New' text is empty", locale),
				Detail:    "Users expect release notes describing changes in each update.",
				Fix:       "Add release notes describing what changed in this version.",
			})
		}

		// Promotional text: length limit
		pt := strings.TrimSpace(attrs.PromotionalText)
		if pt != "" && utf8.RuneCountInString(pt) > maxPromotionalTextLength {
			*findings = append(*findings, Finding{
				Tier:      TierMetadata,
				Severity:  SeverityBlock,
				Guideline: "2.3",
				Title:     fmt.Sprintf("[%s] Promotional text exceeds %d character limit (%d chars)", locale, maxPromotionalTextLength, utf8.RuneCountInString(pt)),
				Detail:    "Promotional text has a 170-character limit.",
				Fix:       fmt.Sprintf("Shorten your promotional text to %d characters.", maxPromotionalTextLength),
			})
		}

		// Support URL: required
		if strings.TrimSpace(attrs.SupportURL) == "" {
			*findings = append(*findings, Finding{
				Tier:      TierMetadata,
				Severity:  SeverityBlock,
				Guideline: "1.5",
				Title:     fmt.Sprintf("[%s] Support URL is missing", locale),
				Detail:    "A support URL is required for App Store submission.",
				Fix:       "Add a support URL pointing to your help/contact page.",
			})
		}
	}

	return nil
}

// checkScreenshots verifies screenshot sets exist.
func checkScreenshots(ctx context.Context, client *asc.Client, appID string, findings *[]Finding) error {
	versions, err := client.GetAppStoreVersions(appID)
	if err != nil {
		return err
	}
	if len(versions) != 1 {
		return fmt.Errorf("submission version is not pinned")
	}
	app, err := client.GetApp(appID)
	if err != nil {
		return err
	}
	localizations, err := client.GetVersionLocalizations(versions[0].ID)
	if err != nil {
		return err
	}
	primaryFound := false
	for _, loc := range localizations {
		primary := loc.Attributes.Locale == app.Attributes.PrimaryLocale
		if primary {
			primaryFound = true
		}
		sets, err := client.GetScreenshotSets(loc.ID)
		if err != nil {
			return fmt.Errorf("[%s] screenshot sets: %w", loc.Attributes.Locale, err)
		}
		count := 0
		for _, set := range sets {
			shots, err := client.GetScreenshots(set.ID)
			if err != nil {
				return fmt.Errorf("[%s] screenshots: %w", loc.Attributes.Locale, err)
			}
			count += len(shots)
			if len(shots) > 10 {
				*findings = append(*findings, Finding{Tier: TierMetadata, Severity: SeverityBlock, Title: "Too many screenshots in set " + set.ID, Detail: fmt.Sprintf("[%s] %d screenshots in %s", loc.Attributes.Locale, len(shots), set.Attributes.ScreenshotDisplayType)})
			}
			for _, shot := range shots {
				state := ""
				if shot.Attributes.AssetDeliveryState != nil {
					state = shot.Attributes.AssetDeliveryState.State
				}
				if state != "COMPLETE" {
					severity := SeverityBlock
					confidence := "high"
					if state == "" {
						severity = SeverityWarn
						confidence = "none"
					}
					*findings = append(*findings, Finding{Tier: TierMetadata, Severity: severity, Confidence: confidence, Title: "Screenshot delivery not confirmed: " + shot.ID, Detail: fmt.Sprintf("[%s] %s delivery state: %q", loc.Attributes.Locale, shot.Attributes.FileName, state), Fix: "Wait for COMPLETE or resolve the upload error in App Store Connect."})
					if state == "" {
						return fmt.Errorf("missing asset delivery state for screenshot %s", shot.ID)
					}
				}
			}
		}
		if count == 0 {
			severity := SeverityWarn
			detail := "No local screenshots; verify primary-locale fallback is appropriate."
			if primary {
				severity = SeverityBlock
				detail = "Primary localization has no screenshots."
			}
			*findings = append(*findings, Finding{Tier: TierMetadata, Severity: severity, Guideline: "2.3.3", Confidence: "high", Title: "No screenshots: " + loc.Attributes.Locale, Detail: detail, Fix: "Check required device sizes and locale fallback against Apple's screenshot specifications."})
		}
	}
	if !primaryFound {
		return fmt.Errorf("primary locale %q was not returned; screenshot coverage incomplete", app.Attributes.PrimaryLocale)
	}
	return nil
}

// selectedBuild deliberately follows the version relationship, not upload order.
func selectedBuild(client *asc.Client, appID string) (*asc.Build, error) {
	versions, err := client.GetAppStoreVersions(appID)
	if err != nil {
		return nil, err
	}
	if len(versions) != 1 {
		return nil, fmt.Errorf("submission version is not pinned")
	}
	return client.GetVersionBuild(versions[0].ID)
}

func checkBuildProcessed(ctx context.Context, client *asc.Client, appID string, findings *[]Finding) error {
	build, err := selectedBuild(client, appID)
	if err != nil {
		return err
	}
	if build == nil {
		*findings = append(*findings, Finding{Tier: TierMetadata, Severity: SeverityBlock, Confidence: "high", Title: "No build attached to selected version", Detail: "An uploaded build does not count until selected for this App Store version.", Fix: "Select the intended build in App Store Connect."})
		return nil
	}
	if build.Attributes.ProcessingState != "VALID" || build.Attributes.Expired {
		*findings = append(*findings, Finding{Tier: TierMetadata, Severity: SeverityBlock, Confidence: "high", Title: "Attached build is not usable", Detail: fmt.Sprintf("Build %s (%s), processing=%s, expired=%t", build.Attributes.Version, build.ID, build.Attributes.ProcessingState, build.Attributes.Expired), Fix: "Select a valid, unexpired build for this version."})
	}
	return nil
}

// checkAgeRating verifies age rating has been declared.
func checkAgeRating(ctx context.Context, client *asc.Client, appID string, findings *[]Finding) error {
	infos, err := client.GetAppInfos(appID)
	if err != nil {
		return err
	}

	if len(infos) == 0 {
		return fmt.Errorf("no app info returned; age rating is unverified")
	}

	versions, err := client.GetAppStoreVersions(appID)
	if err != nil {
		return err
	}
	if len(versions) != 1 {
		return fmt.Errorf("submission version is not pinned")
	}
	var matching []asc.AppInfo
	for _, info := range infos {
		if info.Attributes.AppStoreState == versions[0].Attributes.AppStoreState {
			matching = append(matching, info)
		}
	}
	if len(matching) != 1 {
		return fmt.Errorf("cannot identify one app-info record matching selected version state; age rating is unverified")
	}
	info := matching[0]
	if info.Attributes.AppStoreAgeRating == "" {
		return fmt.Errorf("age rating not returned for matching app info %s; confirm the questionnaire in App Store Connect", info.ID)
	}

	return nil
}

// checkEncryption verifies encryption compliance status.
func checkEncryption(ctx context.Context, client *asc.Client, appID string, findings *[]Finding) error {
	build, err := selectedBuild(client, appID)
	if err != nil {
		return err
	}
	if build == nil {
		return fmt.Errorf("cannot verify encryption without an attached build")
	}
	if build.Attributes.UsesNonExemptEncryption == nil {
		*findings = append(*findings, Finding{Tier: TierMetadata, Severity: SeverityWarn, Confidence: "none", Title: "Attached build encryption declaration unverified", Detail: "No encryption declaration was returned for attached build " + build.ID, Fix: "Confirm export compliance for this specific build in App Store Connect."})
	}
	return nil
}

func checkScreenshotDimensions(ctx context.Context, client *asc.Client, appID string, findings *[]Finding) error {
	versions, err := client.GetAppStoreVersions(appID)
	if err != nil {
		return err
	}
	if len(versions) != 1 {
		return fmt.Errorf("submission version is not pinned")
	}
	localizations, err := client.GetVersionLocalizations(versions[0].ID)
	if err != nil {
		return err
	}
	var gaps []string
	for _, loc := range localizations {
		sets, err := client.GetScreenshotSets(loc.ID)
		if err != nil {
			return err
		}
		for _, set := range sets {
			shots, err := client.GetScreenshots(set.ID)
			if err != nil {
				return err
			}
			for _, shot := range shots {
				if shot.Attributes.ImageAsset == nil {
					gaps = append(gaps, "missing dimensions: "+shot.ID)
					continue
				}
				im := shot.Attributes.ImageAsset
				valid, known := screenshots.ValidSize(set.Attributes.ScreenshotDisplayType, im.Width, im.Height)
				if !known {
					gaps = append(gaps, "unrecognized display type: "+set.Attributes.ScreenshotDisplayType)
					continue
				}
				if !valid {
					*findings = append(*findings, Finding{Tier: TierMetadata, Severity: SeverityBlock, Confidence: "high", Guideline: "2.3.3", Title: "Screenshot dimensions mismatch: " + shot.ID, Detail: fmt.Sprintf("[%s] %s: %dx%d for %s", loc.Attributes.Locale, shot.Attributes.FileName, im.Width, im.Height, set.Attributes.ScreenshotDisplayType), Fix: "Check accepted sizes at " + screenshots.Specifications})
				}
			}
		}
	}
	if len(gaps) > 0 {
		return fmt.Errorf("dimension checks incomplete: %s", strings.Join(gaps, "; "))
	}
	return nil
}

// checkTestFlightExternal checks if external TestFlight testing is configured.
func checkTestFlightExternal(ctx context.Context, client *asc.Client, appID string, findings *[]Finding) error {
	groups, err := client.GetBetaGroups(appID)
	if err != nil {
		// API access failure is unverified coverage.
		return err
	}

	hasExternal := false
	for _, g := range groups {
		if !g.Attributes.IsInternalGroup {
			hasExternal = true
			break
		}
	}

	if !hasExternal {
		*findings = append(*findings, Finding{
			Tier:      TierMetadata,
			Severity:  SeverityInfo,
			Guideline: "2.2",
			Title:     "No external TestFlight group configured",
			Detail:    "External beta testing helps catch issues before App Review. No external beta groups were found.",
			Fix:       "Create an external TestFlight group in App Store Connect → TestFlight to get real user feedback before submission.",
		})
	}

	return nil
}

// checkTerritoryAvailability verifies the app is available in territories.
func checkTerritoryAvailability(ctx context.Context, client *asc.Client, appID string, findings *[]Finding) error {
	territories, err := client.GetAppAvailability(appID)
	if err != nil {
		return err
	}

	if len(territories) == 0 {
		*findings = append(*findings, Finding{
			Tier:     TierMetadata,
			Severity: SeverityBlock,
			Title:    "App not available in any territory",
			Detail:   "The app has no territory availability configured.",
			Fix:      "Set territory availability in App Store Connect → Pricing and Availability.",
		})
	} else if len(territories) < 5 {
		*findings = append(*findings, Finding{
			Tier:     TierMetadata,
			Severity: SeverityInfo,
			Title:    fmt.Sprintf("App available in only %d territories", len(territories)),
			Detail:   "Your app is available in very few territories. Consider expanding for wider reach.",
			Fix:      "Review territory availability in App Store Connect → Pricing and Availability.",
		})
	}

	return nil
}

// checkPricingConsistency verifies pricing is set up.
func checkPricingConsistency(ctx context.Context, client *asc.Client, appID string, findings *[]Finding) error {
	prices, err := client.GetAppPriceSchedule(appID)
	if err != nil {
		return err
	}

	if len(prices) == 0 {
		*findings = append(*findings, Finding{
			Tier:     TierMetadata,
			Severity: SeverityWarn,
			Title:    "No price schedule configured",
			Detail:   "No pricing information found. Ensure your app's price (including Free) is explicitly set.",
			Fix:      "Set pricing in App Store Connect → Pricing and Availability.",
		})
	}

	return nil
}

// checkAppNameLength validates the app name length against App Store limits.
func checkAppNameLength(ctx context.Context, client *asc.Client, appID string, findings *[]Finding) error {
	app, err := client.GetApp(appID)
	if err != nil {
		return err
	}

	name := strings.TrimSpace(app.Attributes.Name)
	if utf8.RuneCountInString(name) > maxAppNameLength {
		*findings = append(*findings, Finding{
			Tier:      TierMetadata,
			Severity:  SeverityBlock,
			Guideline: "2.3",
			Title:     fmt.Sprintf("App name exceeds %d character limit (%d chars)", maxAppNameLength, utf8.RuneCountInString(name)),
			Detail:    "App Store app names are limited to 30 characters.",
			Fix:       fmt.Sprintf("Shorten your app name to %d characters or less.", maxAppNameLength),
		})
	}

	return nil
}

// checkURLReachability verifies that support/marketing URLs are reachable.
func checkURLReachability(ctx context.Context, client *asc.Client, appID string, findings *[]Finding) error {
	versions, err := client.GetAppStoreVersions(appID)
	if err != nil || len(versions) == 0 {
		return err
	}

	localizations, err := client.GetVersionLocalizations(versions[0].ID)
	if err != nil || len(localizations) == 0 {
		return err
	}

	httpClient := &http.Client{Timeout: 10 * time.Second}

	for _, loc := range localizations {
		urls := map[string]string{
			"Support URL":   loc.Attributes.SupportURL,
			"Marketing URL": loc.Attributes.MarketingURL,
		}

		for name, url := range urls {
			if url == "" {
				continue
			}
			resp, err := httpClient.Get(url)
			if err != nil || resp.StatusCode >= 400 {
				*findings = append(*findings, Finding{
					Tier:      TierContent,
					Severity:  SeverityWarn,
					Guideline: "2.3",
					Title:     fmt.Sprintf("[%s] %s is unreachable: %s", loc.Attributes.Locale, name, url),
					Detail:    "Apple verifies that URLs in your metadata are accessible during review.",
					Fix:       "Ensure the URL is live and returns a 200 status code.",
				})
			}
			if resp != nil {
				resp.Body.Close()
			}
		}
	}

	return nil
}
