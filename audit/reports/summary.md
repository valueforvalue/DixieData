# DixieData UI/UX Audit — Summary

Routes audited: **39** (13 unique paths × 3 viewports)
Total findings: **355**

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
- `nested-interactive`: 9
- `overflow-x`: 3
- `small-tap-target`: 1

## Per-route snapshot

| Route | Viewport | Status | Load (ms) | A11y types | Visual issues |
|---|---|---:|---:|---:|---:|
| `/` | desktop | 200 | 1014 | 0 | 1 |
| `/calendar` | desktop | 200 | 876 | 0 | 1 |
| `/soldiers` | desktop | 200 | 5857 | 0 | 2 |
| `/browse` | desktop | 200 | 1915 | 2 | 3 |
| `/review-queue` | desktop | 200 | 783 | 0 | 1 |
| `/insights` | desktop | 200 | 1080 | 1 | 2 |
| `/share` | desktop | 200 | 1088 | 0 | 1 |
| `/settings` | desktop | 200 | 1002 | 1 | 2 |
| `/soldiers/new` | desktop | 200 | 1062 | 3 | 2 |
| `/soldiers/47` | desktop | 200 | 1082 | 0 | 1 |
| `/soldiers/47/edit` | desktop | 200 | 1139 | 3 | 2 |
| `/soldiers/35` | desktop | 200 | 1012 | 0 | 1 |
| `/soldiers/35/edit` | desktop | 200 | 1069 | 3 | 2 |
| `/` | tablet | 200 | 1033 | 0 | 1 |
| `/calendar` | tablet | 200 | 916 | 0 | 1 |
| `/soldiers` | tablet | 200 | 5844 | 0 | 2 |
| `/browse` | tablet | 200 | 1887 | 2 | 2 |
| `/review-queue` | tablet | 200 | 752 | 0 | 1 |
| `/insights` | tablet | 200 | 1058 | 1 | 2 |
| `/share` | tablet | 200 | 1043 | 0 | 1 |
| `/settings` | tablet | 200 | 956 | 1 | 2 |
| `/soldiers/new` | tablet | 200 | 1022 | 3 | 2 |
| `/soldiers/47` | tablet | 200 | 994 | 0 | 2 |
| `/soldiers/47/edit` | tablet | 200 | 1021 | 3 | 2 |
| `/soldiers/35` | tablet | 200 | 948 | 0 | 2 |
| `/soldiers/35/edit` | tablet | 200 | 1033 | 3 | 2 |
| `/` | mobile | 200 | 998 | 0 | 1 |
| `/calendar` | mobile | 200 | 903 | 0 | 1 |
| `/soldiers` | mobile | 200 | 5814 | 0 | 2 |
| `/browse` | mobile | 200 | 1463 | 2 | 3 |
| `/review-queue` | mobile | 200 | 728 | 0 | 1 |
| `/insights` | mobile | 200 | 989 | 1 | 2 |
| `/share` | mobile | 200 | 1024 | 0 | 1 |
| `/settings` | mobile | 200 | 890 | 1 | 2 |
| `/soldiers/new` | mobile | 200 | 987 | 3 | 2 |
| `/soldiers/47` | mobile | 200 | 990 | 0 | 1 |
| `/soldiers/47/edit` | mobile | 200 | 992 | 3 | 2 |
| `/soldiers/35` | mobile | 200 | 904 | 0 | 1 |
| `/soldiers/35/edit` | mobile | 200 | 981 | 3 | 2 |

## Worst offenders (routes with most violations)

- **desktop_browse** — 2 a11y types, 3 visual issues
- **desktop_soldier-new** — 3 a11y types, 2 visual issues
- **desktop_soldier-47-edit** — 3 a11y types, 2 visual issues
- **desktop_soldier-35-edit** — 3 a11y types, 2 visual issues
- **tablet_soldier-new** — 3 a11y types, 2 visual issues
- **tablet_soldier-47-edit** — 3 a11y types, 2 visual issues
- **tablet_soldier-35-edit** — 3 a11y types, 2 visual issues
- **mobile_browse** — 2 a11y types, 3 visual issues
- **mobile_soldier-new** — 3 a11y types, 2 visual issues
- **mobile_soldier-47-edit** — 3 a11y types, 2 visual issues