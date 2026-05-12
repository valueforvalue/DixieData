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

	"github.com/valueforvalue/DixieData/internal/appdata"
	"github.com/valueforvalue/DixieData/internal/db"
	"github.com/valueforvalue/DixieData/internal/models"
	"github.com/valueforvalue/DixieData/internal/services"
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
	defer database.Close()

	soldierSvc := services.NewSoldierService(database)
	conn := database.Conn()
	rng := rand.New(rand.NewSource(options.Seed))

	summary := Summary{
		DataDir:  options.DataDir,
		DBPath:   dbPath,
		ImageDir: imageDir,
	}

	for i := 0; i < options.Soldiers; i++ {
		soldier := buildSoldier(rng, i)
		created, err := soldierSvc.Create(soldier)
		if err != nil {
			return Summary{}, fmt.Errorf("create soldier %d: %w", i+1, err)
		}
		summary.Soldiers++

		recordCount := 1 + rng.Intn(3)
		for j := 0; j < recordCount; j++ {
			record := buildRecord(rng, *created, j)
			if err := insertRecord(conn, created.ID, record); err != nil {
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
			if err := insertImage(conn, created.ID, image); err != nil {
				return Summary{}, fmt.Errorf("insert image for soldier %d: %w", created.ID, err)
			}
			summary.Images++
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

func insertRecord(conn *sql.DB, soldierID int64, record models.Record) error {
	_, err := conn.Exec(
		`INSERT INTO records (soldier_id, record_type, app_id, details) VALUES (?, ?, ?, ?)`,
		soldierID,
		record.RecordType,
		record.AppID,
		record.Details,
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
	defer output.Close()
	if err := png.Encode(output, img); err != nil {
		return models.Image{}, err
	}

	return models.Image{
		FileName: fileName,
		FilePath: filepath.Join(relativeDir, fileName),
		Caption:  caption,
	}, nil
}

func insertImage(conn *sql.DB, soldierID int64, image models.Image) error {
	_, err := conn.Exec(
		`INSERT INTO images (soldier_id, file_name, file_path, caption) VALUES (?, ?, ?, ?)`,
		soldierID,
		image.FileName,
		image.FilePath,
		image.Caption,
	)
	return err
}
