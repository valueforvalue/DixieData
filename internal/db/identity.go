package db

import (
	"crypto/rand"
	"database/sql"
	"fmt"
	"strconv"
	"strings"

	"github.com/valueforvalue/DixieData/internal/models"
)

const DefaultNodePrefix = "TDM65"

func NormalizeNodePrefix(prefix string) string {
	trimmed := strings.ToUpper(strings.TrimSpace(prefix))
	if trimmed == "" {
		return DefaultNodePrefix
	}
	return trimmed
}

func (d *DB) SystemConfig(key string) (string, error) {
	var value string
	err := d.conn.QueryRow(`SELECT value FROM system_config WHERE key = ?`, key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return value, nil
}

func (d *DB) SetSystemConfig(key, value string) error {
	_, err := d.conn.Exec(`
		INSERT INTO system_config(key, value)
		VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET
			value = excluded.value,
			updated_at = CURRENT_TIMESTAMP
	`, key, value)
	return err
}

func (d *DB) NodePrefix() (string, error) {
	value, err := d.SystemConfig("node_prefix")
	if err != nil {
		return "", err
	}
	return NormalizeNodePrefix(value), nil
}

func BuildUserNodePrefix(firstName, middleName, lastName string, birthYear int) (string, error) {
	firstInitial := firstPrefixInitial(firstName)
	middleInitial := firstPrefixInitial(middleName)
	lastInitial := firstPrefixInitial(lastName)
	if firstInitial == "" || middleInitial == "" || lastInitial == "" {
		return "", fmt.Errorf("first, middle, and last name are required")
	}
	if birthYear < 1000 || birthYear > 9999 {
		return "", fmt.Errorf("birth year must be four digits")
	}
	return firstInitial + middleInitial + lastInitial + fmt.Sprintf("%02d", birthYear%100), nil
}

func (d *DB) UserIdentity() (models.UserIdentity, error) {
	var identity models.UserIdentity
	var err error
	identity.FirstName, err = d.SystemConfig("user_first_name")
	if err != nil {
		return models.UserIdentity{}, err
	}
	identity.MiddleName, err = d.SystemConfig("user_middle_name")
	if err != nil {
		return models.UserIdentity{}, err
	}
	identity.LastName, err = d.SystemConfig("user_last_name")
	if err != nil {
		return models.UserIdentity{}, err
	}
	birthYear, err := d.SystemConfig("user_birth_year")
	if err != nil {
		return models.UserIdentity{}, err
	}
	if strings.TrimSpace(birthYear) != "" {
		parsed, parseErr := strconv.Atoi(strings.TrimSpace(birthYear))
		if parseErr != nil {
			return models.UserIdentity{}, parseErr
		}
		identity.BirthYear = parsed
	}
	identity.NodePrefix, err = d.NodePrefix()
	if err != nil {
		return models.UserIdentity{}, err
	}
	return identity, nil
}

func (d *DB) IdentitySetupRequired() (bool, error) {
	complete, err := d.SystemConfig("user_identity_complete")
	if err != nil {
		return false, err
	}
	if strings.TrimSpace(complete) == "1" {
		return false, nil
	}
	nodePrefix, err := d.NodePrefix()
	if err != nil {
		return false, err
	}
	var soldierCount int
	if err := d.conn.QueryRow(`SELECT COUNT(*) FROM soldiers`).Scan(&soldierCount); err != nil {
		return false, err
	}
	return soldierCount == 0 && nodePrefix == DefaultNodePrefix, nil
}

func (d *DB) ConfigureUserIdentity(firstName, middleName, lastName string, birthYear int) (models.UserIdentity, error) {
	firstName = strings.TrimSpace(firstName)
	middleName = strings.TrimSpace(middleName)
	lastName = strings.TrimSpace(lastName)
	nodePrefix, err := BuildUserNodePrefix(firstName, middleName, lastName, birthYear)
	if err != nil {
		return models.UserIdentity{}, err
	}
	tx, err := d.conn.Begin()
	if err != nil {
		return models.UserIdentity{}, err
	}
	defer tx.Rollback()

	fullName := strings.TrimSpace(strings.Join([]string{firstName, middleName, lastName}, " "))
	for key, value := range map[string]string{
		"user_first_name":        firstName,
		"user_middle_name":       middleName,
		"user_last_name":         lastName,
		"user_birth_year":        fmt.Sprintf("%04d", birthYear),
		"user_display_name":      fullName,
		"user_identity_complete": "1",
		"node_prefix":            nodePrefix,
	} {
		if _, err := tx.Exec(`
			INSERT INTO system_config(key, value)
			VALUES (?, ?)
			ON CONFLICT(key) DO UPDATE SET
				value = excluded.value,
				updated_at = CURRENT_TIMESTAMP
		`, key, value); err != nil {
			return models.UserIdentity{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return models.UserIdentity{}, err
	}
	return models.UserIdentity{
		FirstName:  firstName,
		MiddleName: middleName,
		LastName:   lastName,
		BirthYear:  birthYear,
		NodePrefix: nodePrefix,
	}, nil
}

func firstPrefixInitial(value string) string {
	for _, r := range strings.ToUpper(strings.TrimSpace(value)) {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			return string(r)
		}
	}
	return ""
}

func NewSyncID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	buf[6] = (buf[6] & 0x0f) | 0x70
	buf[8] = (buf[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", buf[0:4], buf[4:6], buf[6:8], buf[8:10], buf[10:16]), nil
}
