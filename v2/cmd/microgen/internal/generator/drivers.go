package generator

type driverMeta struct {
	Driver    string
	ImportPkg string
	// Opener names the driver Open function. The template calls it with the DSN it
	// has in hand rather than with a flag variable, so a generated project can open
	// the connection wherever the string is in scope — the main function, or a
	// module it hands to the Host.
	Opener     string
	DefaultDSN string
	ConfigDSN  string
}

var supportedDrivers = map[string]driverMeta{
	"mysql": {
		Driver:     "mysql",
		ImportPkg:  "gorm.io/driver/mysql",
		Opener:     "mysql.Open",
		DefaultDSN: "root:password@tcp(127.0.0.1:3306)/{svcname}?charset=utf8mb4&parseTime=True&loc=Local",
		ConfigDSN:  "root:password@tcp(127.0.0.1:3306)/{svcname}?charset=utf8mb4&parseTime=True&loc=Local",
	},
	"postgres": {
		Driver:     "postgres",
		ImportPkg:  "gorm.io/driver/postgres",
		Opener:     "postgres.Open",
		DefaultDSN: "host=127.0.0.1 user=postgres password=password dbname={svcname} port=5432 sslmode=disable",
		ConfigDSN:  "host=127.0.0.1 user=postgres password=password dbname={svcname} port=5432 sslmode=disable",
	},
	"sqlserver": {
		Driver:     "sqlserver",
		ImportPkg:  "gorm.io/driver/sqlserver",
		Opener:     "sqlserver.Open",
		DefaultDSN: "sqlserver://sa:password@127.0.0.1:1433?database={svcname}",
		ConfigDSN:  "sqlserver://sa:password@127.0.0.1:1433?database={svcname}",
	},
	"clickhouse": {
		Driver:     "clickhouse",
		ImportPkg:  "gorm.io/driver/clickhouse",
		Opener:     "clickhouse.Open",
		DefaultDSN: "tcp://127.0.0.1:9000?database={svcname}&username=default&password=&read_timeout=10&write_timeout=20",
		ConfigDSN:  "tcp://127.0.0.1:9000?database={svcname}&username=default&password=&read_timeout=10&write_timeout=20",
	},
	"sqlite": {
		Driver:     "sqlite",
		ImportPkg:  "gorm.io/driver/sqlite",
		Opener:     "sqlite.Open",
		DefaultDSN: "app.db",
		ConfigDSN:  "app.db",
	},
}
