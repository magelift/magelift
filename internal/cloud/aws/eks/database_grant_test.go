package eks

import (
	"strings"
	"testing"
)

func TestDatabaseGrantStatementEscapesMySQLValues(t *testing.T) {
	statement, err := databaseGrantStatement("store`catalog", "mage'o")
	if err != nil {
		t.Fatal(err)
	}
	want := "GRANT ALL PRIVILEGES ON `store``catalog`.* TO 'mage''o'@'%';"
	if statement != want {
		t.Fatalf("database grant statement = %q, want %q", statement, want)
	}
}

func TestDatabaseGrantStatementRejectsInvalidValues(t *testing.T) {
	for _, value := range []string{"", "store\nname", "mage\x00user"} {
		if _, err := databaseGrantStatement(value, "magento"); err == nil {
			t.Fatalf("database name %q was accepted", value)
		}
	}
	if _, err := databaseGrantStatement("magento", "mage\nuser"); err == nil || !strings.Contains(err.Error(), "control") {
		t.Fatalf("invalid username error = %v", err)
	}
}
