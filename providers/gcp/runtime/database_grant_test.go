package runtime

import "testing"

func TestDatabaseGrantStatementEscapesMySQLValues(t *testing.T) {
	statement, err := databaseGrantStatement("store`catalog", "mage'o")
	if err != nil {
		t.Fatal(err)
	}
	want := "GRANT ALL PRIVILEGES ON `store``catalog`.* TO 'mage''o'@'%';"
	if statement != want {
		t.Fatalf("grant statement = %q, want %q", statement, want)
	}
}

func TestDatabaseGrantStatementRejectsInvalidValues(t *testing.T) {
	for _, value := range []struct {
		name     string
		database string
		username string
	}{
		{name: "missing database", username: "magento"},
		{name: "missing username", database: "magento"},
		{name: "control character", database: "magento\nprod", username: "magento"},
	} {
		t.Run(value.name, func(t *testing.T) {
			if _, err := databaseGrantStatement(value.database, value.username); err == nil {
				t.Fatal("expected invalid grant inputs to fail")
			}
		})
	}
}
