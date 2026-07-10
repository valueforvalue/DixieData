// Package seed generates deterministic sample soldiers, records, and images into a DixieData data directory.
package seed

import (
	"database/sql"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/valueforvalue/DixieData/internal/appdata"
	"github.com/valueforvalue/DixieData/internal/buildinfo"
	"github.com/valueforvalue/DixieData/internal/db"
	"github.com/valueforvalue/DixieData/internal/debug"
	"github.com/valueforvalue/DixieData/internal/models"
	"github.com/valueforvalue/DixieData/internal/records"
)

const (
	defaultSoldierCount = 250
	defaultSeed         = 1865
)

var (
	firstNames    = []string{"James", "William", "John", "Thomas", "Robert", "Samuel", "George", "Henry", "Joseph", "Charles", "Andrew", "Edward", "Benjamin", "Francis", "Nathaniel", "Lewis", "Richard", "Elijah", "Walter", "Jasper"}
	middleNames   = []string{"Allen", "Bell", "Clay", "Davis", "Edward", "Franklin", "Gray", "Henry", "Isaac", "Jasper", "Knox", "Lee", "Morgan", "Nathan", "Otis", "Perry", "Quincy", "Reuben", "Silas", "Thomas"}
	lastNames     = []string{"Carter", "Walker", "Hughes", "Bennett", "Foster", "McDaniel", "Pritchard", "Hawkins", "Turner", "Coleman", "Whitfield", "Mercer", "Dawson", "Reed", "Calhoun", "Harper", "Tate", "McBride", "Boone", "Abernathy"}
	ranks         = []string{"Private", "Corporal", "Sergeant", "Lieutenant", "Captain", "Major", "Colonel"}
	units         = []string{"1st Georgia Infantry", "4th Alabama Cavalry", "7th Texas Infantry", "12th Virginia Artillery", "15th Tennessee Infantry", "18th Mississippi Cavalry", "22nd North Carolina Infantry", "31st Louisiana Infantry", "3rd Arkansas Mounted Rifles", "5th South Carolina Infantry"}
	states        = []string{"Georgia", "Alabama", "Virginia", "Texas", "Mississippi", "Tennessee", "North Carolina", "South Carolina", "Louisiana", "Arkansas"}
	counties      = []string{"Madison County", "Jefferson County", "Franklin County", "Randolph County", "Monroe County", "Jackson County", "Warren County", "Marion County", "Lee County", "Greene County"}
	cemeteries    = []string{"Oakwood Cemetery, Richmond", "Magnolia Cemetery, Mobile", "Hollywood Cemetery, Richmond", "Elmwood Cemetery, Memphis", "Rose Hill Cemetery, Macon", "Confederate Rest, Helena", "Stonewall Cemetery, Winchester", "Greenwood Cemetery, New Orleans"}
	recordTypes   = []string{"Service Record", "Hospital Ledger", "Parole Note", "Pension Application", "Unit Roster", "Burial Ledger"}
	recordDetails = []string{
		"Filed from county records with marginal notes on service and discharge.",
		"Lists service dates, reported location, and clerk remarks preserved in the archive copy.",
		"Compiled summary prepared for local memorial indexing and cemetery cross-reference.",
		"Contains pension-era testimony transcribed into the registry abstract.",
		"Abstracted from adjutant returns and postwar veterans association notes.",
	}
	imageCaptions = []string{
		"Simulated portrait plate",
		"Simulated gravesite marker rubbing",
		"Simulated service card scan",
		"Simulated pension ledger excerpt",
		"Simulated regimental roster clipping",
	}

	// v58-v65 surface: event kinds, article titles, tag names for
	// the post-v58 entity seeding (issue #447).
	eventKinds = []string{"Battle", "Skirmish", "Siege", "Campaign", "Raid", "Muster"}
	eventDescs  = []string{
		"Engagement between opposing forces near the county seat. Casualty reports vary by source.",
		"Small-scale action involving local militia and a detached cavalry company.",
		"Fortification investment lasting several weeks; supply lines were cut early.",
		"Multi-week movement through contested territory with intermittent contact.",
		"Swift mounted operation targeting a supply depot behind enemy lines.",
		"Formal assembly of the regiment for inspection and payroll distribution.",
	}
	articleTitles = []string{
		"The Road to Manassas: A Prelude",
		"Camp Life in the Army of Northern Virginia",
		"Letters Home: A Soldier's Correspondence",
		"After the Surrender: Reconstruction in the South",
		"The Role of Cavalry in the Western Theater",
	}
	articleBodies = []string{
		"In the spring of 1861, the gathering of forces along Bull Run marked the beginning of a long and bitter conflict. Soldiers from both sides arrived with high spirits and little understanding of what lay ahead.",
		"Daily life in camp revolved around drill, guard duty, and the constant search for adequate rations. Men wrote letters home describing the monotony punctuated by moments of sheer terror.",
		"My dearest Martha, I take up my pen this evening to tell you that I am well, though the march has been hard. We crossed the river at dawn and made camp in a pine grove.",
		"The years following the war brought hardship and hope in equal measure. Communities rebuilt, families reunited, and the long process of healing began.",
		"Mounted units played a decisive role in reconnaissance, screening, and raiding operations throughout the western campaigns. Their mobility often determined the outcome before infantry ever engaged.",
	}
	tagNames = []string{
		"Wounded", "POW", "Deserter", "Promoted", "Transferred",
		"KIA", "Died of Disease", "Paroled", "Enlisted", "Conscript",
	}
)

type Options struct {
	DataDir  string
	Soldiers int
	Seed     int64
	Reset    bool
}

type Summary struct {
	DataDir  string
	DBPath   string
	ImageDir string
	Soldiers int
	Records  int
	Images   int
	// v58-v65 surface (issue #447)
	Events        int
	EventLinks    int
	EventSources  int
	Articles      int
	ArticleRefs   int
	Tags          int
	PersonRecordTags int
}

func Generate(options Options) (Summary, error) {
	options = normalizeOptions(options)
	if strings.TrimSpace(options.DataDir) == "" {
		return Summary{}, errors.New("data directory is required")
	}
	if options.Soldiers <= 0 {
		return Summary{}, errors.New("soldier count must be greater than zero")
	}

	dbPath := filepath.Join(options.DataDir, "dixiedata.db")
	imageDir := filepath.Join(options.DataDir, "images")

	if options.Reset {
		if err := resetData(dbPath, imageDir); err != nil {
			return Summary{}, err
		}
	}

	if err := os.MkdirAll(imageDir, 0o755); err != nil {
		return Summary{}, fmt.Errorf("create image directory: %w", err)
	}

	database, err := db.Open(options.DataDir)
	if err != nil {
		return Summary{}, fmt.Errorf("open database: %w", err)
	}
	// Issue #449 slice 2: wrap the close in a closure so the
	// *DB.Close fires before Generate returns. The bare
	// `defer Close()` form defers the call to a returned
	// function value, which doesn't run until the enclosing
	// frame is gone — too late for the test caller that
	// immediately tries to RemoveAll the testtemp dir.
	defer func() { _ = database.Close() }()

	soldierSvc := records.NewSoldierService(database)
	conn := database.Conn()
	rng := rand.New(rand.NewSource(options.Seed))

	summary := Summary{
		DataDir:  options.DataDir,
		DBPath:   dbPath,
		ImageDir: imageDir,
	}

	for i := 0; i < options.Soldiers; i++ {
		soldier := buildSoldier(rng, i)
		// Issue #377 slice 2: stamp the import path so future
		// "where did this row come from?" investigations can
		// attribute the row to the bulk seed importer. The
		// service-layer default already covers empty values, but
		// explicit stamping documents the intent at the call site
		// and survives any future defaulting change.
		soldier.CreatedByImportPath = "seed"
		created, err := soldierSvc.Create(soldier)
		if err != nil {
			return Summary{}, fmt.Errorf("create soldier %d: %w", i+1, err)
		}
		summary.Soldiers++

		recordCount := 1 + rng.Intn(3)
		for j := 0; j < recordCount; j++ {
			record := buildRecord(rng, *created, j)
			// Issue #447: stamp sort_order (form-array index)
			// so the v63 read path (ORDER BY sort_order, id)
			// is exercised.
			record.SortOrder = int64(j)
			if err := insertRecord(conn, *created, record); err != nil {
				return Summary{}, fmt.Errorf("create record for soldier %d: %w", created.ID, err)
			}
			summary.Records++
		}

		imageCount := 1 + rng.Intn(3)
		for j := 0; j < imageCount; j++ {
			image, err := createImage(options.DataDir, rng, *created, j)
			if err != nil {
				return Summary{}, fmt.Errorf("create image for soldier %d: %w", created.ID, err)
			}
			if err := insertImage(conn, *created, image); err != nil {
				return Summary{}, fmt.Errorf("insert image for soldier %d: %w", created.ID, err)
			}
			summary.Images++
		}
	}

	// Issue #447: seed v58-v65 surface — Event Records, Articles,
	// Tags, and their junction tables. Gated on schema version so
	// pre-v58 dev archives stay unaffected.
	var schemaVersion int
	if err := conn.QueryRow(`PRAGMA user_version`).Scan(&schemaVersion); err != nil {
		return Summary{}, fmt.Errorf("read schema version: %w", err)
	}
	if schemaVersion >= 58 {
		// Collect created soldier IDs for linking.
		soldierIDs, err := loadSoldierIDs(conn)
		if err != nil {
			return Summary{}, fmt.Errorf("load soldier IDs: %w", err)
		}

		// Tags: create all tags first, then assign to soldiers.
		tagIDs, err := seedTags(conn, rng, &summary)
		if err != nil {
			return Summary{}, fmt.Errorf("seed tags: %w", err)
		}
		if err := seedPersonRecordTags(conn, rng, soldierIDs, tagIDs, &summary); err != nil {
			return Summary{}, fmt.Errorf("seed person_record_tags: %w", err)
		}

		// Event Records + links + sources.
		eventIDs, err := seedEvents(conn, rng, &summary)
		if err != nil {
			return Summary{}, fmt.Errorf("seed events: %w", err)
		}
		if err := seedEventPersonLinks(conn, rng, eventIDs, soldierIDs, &summary); err != nil {
			return Summary{}, fmt.Errorf("seed event_person_links: %w", err)
		}
		if err := seedEventSources(conn, rng, eventIDs, &summary); err != nil {
			return Summary{}, fmt.Errorf("seed event_sources: %w", err)
		}

		// Articles + refs.
		articleIDs, err := seedArticles(conn, rng, &summary)
		if err != nil {
			return Summary{}, fmt.Errorf("seed articles: %w", err)
		}
		if err := seedArticleRefs(conn, rng, articleIDs, soldierIDs, &summary); err != nil {
			return Summary{}, fmt.Errorf("seed article_refs: %w", err)
		}
	}

	return summary, nil
}

func normalizeOptions(options Options) Options {
	if options.Soldiers == 0 {
		options.Soldiers = defaultSoldierCount
	}
	if options.Seed == 0 {
		options.Seed = defaultSeed
	}
	return options
}

func resetData(dbPath, imageDir string) error {
	if err := os.Remove(dbPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove existing database: %w", err)
	}
	for _, suffix := range []string{"-shm", "-wal"} {
		if err := os.Remove(dbPath + suffix); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove existing sqlite sidecar: %w", err)
		}
	}
	if err := os.RemoveAll(imageDir); err != nil {
		return fmt.Errorf("remove existing generated images: %w", err)
	}
	return nil
}

func buildSoldier(rng *rand.Rand, index int) models.Soldier {
	firstName := firstNames[rng.Intn(len(firstNames))]
	middleName := middleNames[rng.Intn(len(middleNames))]
	lastName := fmt.Sprintf("%s %s", lastNames[rng.Intn(len(lastNames))], string(rune('A'+(index%26))))
	state := states[rng.Intn(len(states))]
	county := counties[rng.Intn(len(counties))]
	month := 1 + rng.Intn(12)
	day := 1 + rng.Intn(28)
	rankInIndex := rng.Intn(len(ranks))
	rankOutIndex := rankInIndex + rng.Intn(len(ranks)-rankInIndex)
	rankIn := ranks[rankInIndex]
	rankOut := ranks[rankOutIndex]
	if rng.Intn(5) == 0 {
		day = 0
	}

	soldier := models.Soldier{
		PensionID:     fmt.Sprintf("P%05d", 10000+index),
		ApplicationID: fmt.Sprintf("A%05d", 10000+index),
		FirstName:     firstName,
		MiddleName:    middleName,
		LastName:      lastName,
		Rank:          rankOut,
		RankIn:        rankIn,
		RankOut:       rankOut,
		Unit:          units[rng.Intn(len(units))],
		PensionState:  state,
		DeathYear:     1861 + rng.Intn(5),
		DeathMonth:    month,
		DeathDay:      day,
		BirthInfo:     fmt.Sprintf("Born %d in %s, %s.", 1818+rng.Intn(25), county, state),
		BuriedIn:      cemeteries[rng.Intn(len(cemeteries))],
		Notes:         fmt.Sprintf("Generated test entry %03d for UI and export testing.", index+1),
	}

	return soldier
}

func buildRecord(rng *rand.Rand, soldier models.Soldier, index int) models.Record {
	recordType := recordTypes[rng.Intn(len(recordTypes))]
	detail := recordDetails[rng.Intn(len(recordDetails))]
	return models.Record{
		RecordType: recordType,
		AppID:      fmt.Sprintf("APP-%06d-%02d", soldier.ID, index+1),
		Details: fmt.Sprintf(
			"%s %s %s. %s",
			soldier.Rank,
			soldier.FirstName,
			soldier.LastName,
			detail,
		),
	}
}

func insertRecord(conn *sql.DB, soldier models.Soldier, record models.Record) error {
	syncID, err := db.NewSyncID()
	if err != nil {
		return err
	}
	// Issue #447: include sort_order (v63 read path) on every
	// seeded record. Provenance columns (created_by_version,
	// created_by_import_path) only exist on soldiers, not records.
	_, err = conn.Exec(
		`INSERT INTO records (sync_id, person_record_id, person_sync_id, record_type, app_id, details, sort_order) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		syncID,
		soldier.ID,
		soldier.SyncID,
		record.RecordType,
		record.AppID,
		record.Details,
		record.SortOrder,
	)
	return err
}

func createImage(dataDir string, rng *rand.Rand, soldier models.Soldier, index int) (models.Image, error) {
	caption := imageCaptions[rng.Intn(len(imageCaptions))]
	recordDir, relativeDir := appdata.RecordImageDir(dataDir, soldier.DisplayID)
	if err := os.MkdirAll(recordDir, 0o755); err != nil {
		return models.Image{}, err
	}

	fileName := fmt.Sprintf("generated-%02d.png", index+1)
	filePath := filepath.Join(recordDir, fileName)

	img := image.NewRGBA(image.Rect(0, 0, 800, 500))
	base := color.RGBA{R: uint8(40 + rng.Intn(90)), G: uint8(30 + rng.Intn(50)), B: uint8(70 + rng.Intn(100)), A: 255}
	highlight := color.RGBA{R: uint8(180 + rng.Intn(60)), G: uint8(120 + rng.Intn(60)), B: uint8(60 + rng.Intn(40)), A: 255}
	shadow := color.RGBA{R: 20, G: 20, B: 35, A: 255}

	for y := 0; y < 500; y++ {
		for x := 0; x < 800; x++ {
			switch {
			case x > 40 && x < 760 && y > 40 && y < 460:
				img.Set(x, y, base)
			default:
				img.Set(x, y, shadow)
			}
			if (x/40+y/40)%5 == 0 && x > 80 && x < 720 && y > 80 && y < 420 {
				img.Set(x, y, highlight)
			}
		}
	}

	output, err := os.Create(filePath)
	if err != nil {
		return models.Image{}, err
	}
	defer func() { debug.DeferCloseLog(output, "createImage.output")() }()
	if err := png.Encode(output, img); err != nil {
		return models.Image{}, err
	}

	return models.Image{
		FileName: fileName,
		FilePath: filepath.Join(relativeDir, fileName),
		Caption:  caption,
	}, nil
}

func insertImage(conn *sql.DB, soldier models.Soldier, image models.Image) error {
	syncID, err := db.NewSyncID()
	if err != nil {
		return err
	}
	_, err = conn.Exec(
		`INSERT INTO images (sync_id, person_record_id, person_sync_id, file_name, file_path, caption) VALUES (?, ?, ?, ?, ?, ?)`,
		syncID,
		soldier.ID,
		soldier.SyncID,
		image.FileName,
		image.FilePath,
		image.Caption,
	)
	return err
}

// --- v58-v65 surface helpers (issue #447) ---

// loadSoldierIDs returns every soldier.id from the seeded archive
// so the event/article link helpers can pick random targets.
func loadSoldierIDs(conn *sql.DB) ([]int64, error) {
	rows, err := conn.Query(`SELECT id FROM soldiers ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// seedTags creates all tagNames rows and returns their IDs.
func seedTags(conn *sql.DB, rng *rand.Rand, summary *Summary) ([]int64, error) {
	var ids []int64
	for _, name := range tagNames {
		res, err := conn.Exec(
			`INSERT OR IGNORE INTO tags (name, normalized_name) VALUES (?, ?)`,
			name, strings.ToLower(name),
		)
		if err != nil {
			return nil, err
		}
		id, err := res.LastInsertId()
		if err != nil {
			return nil, err
		}
		// INSERT OR IGNORE returns 0 for existing rows; fetch the real ID.
		if id == 0 {
			if err := conn.QueryRow(`SELECT id FROM tags WHERE normalized_name = ?`, strings.ToLower(name)).Scan(&id); err != nil {
				return nil, err
			}
		}
		ids = append(ids, id)
		summary.Tags++
	}
	return ids, nil
}

// seedPersonRecordTags assigns 3-5 random tags to each soldier.
func seedPersonRecordTags(conn *sql.DB, rng *rand.Rand, soldierIDs, tagIDs []int64, summary *Summary) error {
	for _, sid := range soldierIDs {
		n := 3 + rng.Intn(3) // 3-5 tags per soldier
		// Shuffle tagIDs and pick the first n.
		perm := rng.Perm(len(tagIDs))
		for j := 0; j < n && j < len(perm); j++ {
			tid := tagIDs[perm[j]]
			if _, err := conn.Exec(
				`INSERT OR IGNORE INTO person_record_tags (person_id, tag_id) VALUES (?, ?)`,
				sid, tid,
			); err != nil {
				return err
			}
			summary.PersonRecordTags++
		}
	}
	return nil
}

// seedEvents creates N Event Record rows (entry_type='event') and
// returns their IDs. N = ~20% of the soldier count, min 2.
func seedEvents(conn *sql.DB, rng *rand.Rand, summary *Summary) ([]int64, error) {
	n := summary.Soldiers / 5
	if n < 2 {
		n = 2
	}
	var ids []int64
	now := time.Now().UTC().Format("2006-01-02 15:04:05")
	for i := 0; i < n; i++ {
		kind := eventKinds[rng.Intn(len(eventKinds))]
		desc := eventDescs[rng.Intn(len(eventDescs))]
		month := 1 + rng.Intn(12)
		day := 1 + rng.Intn(28)
		year := 1861 + rng.Intn(5)
		beginDate := fmt.Sprintf("%02d/%02d/%d", month, day, year)
		endDate := fmt.Sprintf("%02d/%02d/%d", month, day+1+rng.Intn(3), year)
		syncID, err := db.NewSyncID()
		if err != nil {
			return nil, err
		}
		displayID := fmt.Sprintf("EVT-%06d", 10000+i)
		res, err := conn.Exec(
			`INSERT INTO soldiers (sync_id, display_id, entry_type, kind, begin_date, end_date, description, created_by_version, created_by_import_path, created_at, updated_at) VALUES (?, ?, 'event', ?, ?, ?, ?, ?, 'seed', ?, ?)`,
			syncID, displayID, kind, beginDate, endDate, desc, buildinfo.AppVersion, now, now,
		)
		if err != nil {
			return nil, err
		}
		id, err := res.LastInsertId()
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
		summary.Events++
	}
	return ids, nil
}

// seedEventPersonLinks attaches 1-3 random soldiers to each event.
func seedEventPersonLinks(conn *sql.DB, rng *rand.Rand, eventIDs, soldierIDs []int64, summary *Summary) error {
	for _, eid := range eventIDs {
		n := 1 + rng.Intn(3) // 1-3 soldiers per event
		perm := rng.Perm(len(soldierIDs))
		for j := 0; j < n && j < len(perm); j++ {
			sid := soldierIDs[perm[j]]
			syncID, err := db.NewSyncID()
			if err != nil {
				return err
			}
			if _, err := conn.Exec(
				`INSERT OR IGNORE INTO event_person_links (sync_id, event_id, person_id) VALUES (?, ?, ?)`,
				syncID, eid, sid,
			); err != nil {
				return err
			}
			summary.EventLinks++
		}
	}
	return nil
}

// seedEventSources creates 1-2 source records per event.
func seedEventSources(conn *sql.DB, rng *rand.Rand, eventIDs []int64, summary *Summary) error {
	sourceTypes := []string{"After-Action Report", "Casualty Return", "Morning Report", "Ordnance Return"}
	for _, eid := range eventIDs {
		n := 1 + rng.Intn(2) // 1-2 sources per event
		for j := 0; j < n; j++ {
			syncID, err := db.NewSyncID()
			if err != nil {
				return err
			}
			srcType := sourceTypes[rng.Intn(len(sourceTypes))]
			appID := fmt.Sprintf("SRC-%06d-%02d", eid, j+1)
			if _, err := conn.Exec(
				`INSERT INTO event_sources (sync_id, event_id, record_type, app_id, details, sort_order) VALUES (?, ?, ?, ?, ?, ?)`,
				syncID, eid, srcType, appID, recordDetails[rng.Intn(len(recordDetails))], j,
			); err != nil {
				return err
			}
			summary.EventSources++
		}
	}
	return nil
}

// seedArticles creates 1-2 Article rows and returns their IDs.
func seedArticles(conn *sql.DB, rng *rand.Rand, summary *Summary) ([]int64, error) {
	n := 1 + rng.Intn(2) // 1-2 articles
	var ids []int64
	now := time.Now().UTC().Format("2006-01-02 15:04:05")
	for i := 0; i < n; i++ {
		title := articleTitles[rng.Intn(len(articleTitles))]
		body := articleBodies[rng.Intn(len(articleBodies))]
		displayID := fmt.Sprintf("ART-%06d", 10000+i)
		syncID, err := db.NewSyncID()
		if err != nil {
			return nil, err
		}
		res, err := conn.Exec(
			`INSERT INTO articles (sync_id, display_id, title, subtitle, body_md, body_html, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			syncID, displayID, title, "", body, "<p>"+body+"</p>", now, now,
		)
		if err != nil {
			return nil, err
		}
		id, err := res.LastInsertId()
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
		summary.Articles++
	}
	return ids, nil
}

// seedArticleRefs attaches 2-3 random soldiers to each article.
func seedArticleRefs(conn *sql.DB, rng *rand.Rand, articleIDs, soldierIDs []int64, summary *Summary) error {
	for _, aid := range articleIDs {
		n := 2 + rng.Intn(2) // 2-3 refs per article
		perm := rng.Perm(len(soldierIDs))
		for j := 0; j < n && j < len(perm); j++ {
			sid := soldierIDs[perm[j]]
			if _, err := conn.Exec(
				`INSERT OR IGNORE INTO article_refs (article_id, person_record_id, person_display_id, position) VALUES (?, ?, ?, ?)`,
				aid, sid, fmt.Sprintf("DXD-%05d", sid), j,
			); err != nil {
				return err
			}
			summary.ArticleRefs++
		}
	}
	return nil
}
