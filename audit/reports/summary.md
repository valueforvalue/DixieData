# DixieData UI/UX Audit — Summary

Routes audited: **39** (13 unique paths × 3 viewports)
Total findings: **393**

## A11y violations by severity

- Critical: 282
- Serious: 9
- Moderate: 0
- Minor: 0

## Findings by type

- `label`: 243
- `nav-density`: 39
- `select-name`: 39
- `unlabeled-input`: 21
- `overflow-x`: 15
- `h-scroll`: 13
- `h-scroll-body`: 13
- `nested-interactive`: 9
- `small-tap-target`: 1

## Per-route snapshot

| Route | Viewport | Status | Load (ms) | A11y types | Visual issues |
|---|---|---:|---:|---:|---:|
| `/` | desktop | 200 | 1018 | 0 | 1 |
| `/calendar` | desktop | 200 | 864 | 0 | 1 |
| `/soldiers` | desktop | 200 | 5761 | 0 | 2 |
| `/browse` | desktop | 200 | 1746 | 2 | 3 |
| `/review-queue` | desktop | 200 | 734 | 0 | 1 |
| `/insights` | desktop | 200 | 975 | 1 | 2 |
| `/share` | desktop | 200 | 990 | 0 | 1 |
| `/settings` | desktop | 200 | 910 | 1 | 2 |
| `/soldiers/new` | desktop | 200 | 1011 | 3 | 2 |
| `/soldiers/47` | desktop | 200 | 961 | 0 | 1 |
| `/soldiers/47/edit` | desktop | 200 | 994 | 3 | 2 |
| `/soldiers/35` | desktop | 200 | 930 | 0 | 1 |
| `/soldiers/35/edit` | desktop | 200 | 1005 | 3 | 2 |
| `/` | tablet | 200 | 957 | 0 | 1 |
| `/calendar` | tablet | 200 | 870 | 0 | 1 |
| `/soldiers` | tablet | 200 | 5771 | 0 | 2 |
| `/browse` | tablet | 200 | 1679 | 2 | 2 |
| `/review-queue` | tablet | 200 | 704 | 0 | 1 |
| `/insights` | tablet | 200 | 974 | 1 | 2 |
| `/share` | tablet | 200 | 944 | 0 | 1 |
| `/settings` | tablet | 200 | 851 | 1 | 2 |
| `/soldiers/new` | tablet | 200 | 931 | 3 | 2 |
| `/soldiers/47` | tablet | 200 | 923 | 0 | 2 |
| `/soldiers/47/edit` | tablet | 200 | 940 | 3 | 2 |
| `/soldiers/35` | tablet | 200 | 893 | 0 | 2 |
| `/soldiers/35/edit` | tablet | 200 | 929 | 3 | 2 |
| `/` | mobile | 200 | 956 | 0 | 4 |
| `/calendar` | mobile | 200 | 844 | 0 | 4 |
| `/soldiers` | mobile | 200 | 5745 | 0 | 5 |
| `/browse` | mobile | 200 | 1384 | 2 | 5 |
| `/review-queue` | mobile | 200 | 726 | 0 | 4 |
| `/insights` | mobile | 200 | 938 | 1 | 5 |
| `/share` | mobile | 200 | 949 | 0 | 4 |
| `/settings` | mobile | 200 | 840 | 1 | 5 |
| `/soldiers/new` | mobile | 200 | 975 | 3 | 5 |
| `/soldiers/47` | mobile | 200 | 935 | 0 | 4 |
| `/soldiers/47/edit` | mobile | 200 | 944 | 3 | 5 |
| `/soldiers/35` | mobile | 200 | 904 | 0 | 4 |
| `/soldiers/35/edit` | mobile | 200 | 971 | 3 | 5 |

## Worst offenders (routes with most violations)

- **mobile_soldier-new** — 3 a11y types, 5 visual issues
- **mobile_soldier-47-edit** — 3 a11y types, 5 visual issues
- **mobile_soldier-35-edit** — 3 a11y types, 5 visual issues
- **mobile_browse** — 2 a11y types, 5 visual issues
- **mobile_insights** — 1 a11y types, 5 visual issues
- **mobile_settings** — 1 a11y types, 5 visual issues
- **desktop_browse** — 2 a11y types, 3 visual issues
- **desktop_soldier-new** — 3 a11y types, 2 visual issues
- **desktop_soldier-47-edit** — 3 a11y types, 2 visual issues
- **desktop_soldier-35-edit** — 3 a11y types, 2 visual issues