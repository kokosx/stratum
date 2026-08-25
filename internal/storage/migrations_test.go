package storage

import "testing"

func TestCompareMigrationVersionsUsesNumericPrefix(t *testing.T) {
	if CompareMigrationVersions("100_next.sql", "099_previous.sql") <= 0 {
		t.Fatal("100 must sort after 099")
	}
	if CompareMigrationVersions("009_old.sql", "010_new.sql") >= 0 {
		t.Fatal("009 must sort before 010")
	}
}
