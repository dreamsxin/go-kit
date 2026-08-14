package generator

type driverMeta struct {
	Driver     string
	ImportPkg  string
	OpenCall   string
	DefaultDSN string
	ConfigDSN  string
}

var supportedDrivers = map[string]driverMeta{
	"mysql": {
		Driver:     "mysql",
		ImportPkg:  "gorm.io/driver/mysql",
		OpenCall:   "mysql.Open(*dsn)",
		DefaultDSN: "root:password@tcp(127.0.0.1:3306)/{svcname}?charset=utf8mb4&parseTime=True&loc=Local",
		ConfigDSN:  "root:password@tcp(127.0.0.1:3306)/{svcname}?charset=utf8mb4&parseTime=True&loc=Local",
	},
	"postgres": {
		Driver:     "postgres",
		ImportPkg:  "gorm.io/driver/postgres",
		OpenCall:   "postgres.Open(*dsn)",
		DefaultDSN: "host=127.0.0.1 user=postgres password=password dbname={svcname} port=5432 sslmode=disable",
		ConfigDSN:  "host=127.0.0.1 user=postgres password=password dbname={svcname} port=5432 sslmode=disable",
	},
	"sqlserver": {
		Driver:     "sqlserver",
		ImportPkg:  "gorm.io/driver/sqlserver",
		OpenCall:   "sqlserver.Open(*dsn)",
		DefaultDSN: "sqlserver://sa:password@127.0.0.1:1433?database={svcname}",
		ConfigDSN:  "sqlserver://sa:password@127.0.0.1:1433?database={svcname}",
	},
	"clickhouse": {
		Driver:     "clickhouse",
		ImportPkg:  "gorm.io/driver/clickhouse",
		OpenCall:   "clickhouse.Open(*dsn)",
		DefaultDSN: "tcp://127.0.0.1:9000?database={svcname}&username=default&password=&read_timeout=10&write_timeout=20",
		ConfigDSN:  "tcp://127.0.0.1:9000?database={svcname}&username=default&password=&read_timeout=10&write_timeout=20",
	},
	"sqlite": {
		Driver:     "sqlite",
		ImportPkg:  "gorm.io/driver/sqlite",
		OpenCall:   "sqlite.Open(*dsn)",
		DefaultDSN: "app.db",
		ConfigDSN:  "app.db",
	},
}
