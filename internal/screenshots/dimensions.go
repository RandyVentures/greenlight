package screenshots

const Specifications = "https://developer.apple.com/help/app-store-connect/reference/app-information/screenshot-specifications/"

// Accepted sizes, checked against Apple's specifications on 2026-10-28.
// This is deliberately a supported subset, not an exhaustive policy database.
// Unknown ASC display types must be reported as unverified, never rejected.
var Sizes = map[string][][2]int{
	"APP_IPHONE_67":         {{1260, 2736}, {1290, 2796}, {1320, 2868}},
	"APP_IPHONE_65":         {{1242, 2688}, {1284, 2778}},
	"APP_IPHONE_61":         {{1179, 2556}, {1206, 2622}},
	"APP_IPHONE_58":         {{1125, 2436}, {1170, 2532}, {1080, 2340}},
	"APP_IPHONE_55":         {{1242, 2208}},
	"APP_IPHONE_47":         {{750, 1334}},
	"APP_IPAD_PRO_3GEN_129": {{2048, 2732}, {2064, 2752}},
	"APP_IPAD_PRO_129":      {{2048, 2732}},
	"APP_IPAD_PRO_3GEN_11":  {{1488, 2266}, {1668, 2420}, {1668, 2388}, {1640, 2360}},
	"APP_IPAD_105":          {{1668, 2224}},
	"APP_DESKTOP":           {{1280, 800}, {1440, 900}, {2560, 1600}, {2880, 1800}},
	"APP_APPLE_TV":          {{1920, 1080}, {3840, 2160}},
	"APP_APPLE_VISION_PRO":  {{3840, 2160}},
}

func ValidSize(displayType string, w, h int) (valid, known bool) {
	sizes, known := Sizes[displayType]
	if !known {
		return false, false
	}
	for _, s := range sizes {
		if w == s[0] && h == s[1] || displayType != "APP_DESKTOP" && displayType != "APP_APPLE_TV" && displayType != "APP_APPLE_VISION_PRO" && w == s[1] && h == s[0] {
			return true, true
		}
	}
	return false, true
}
