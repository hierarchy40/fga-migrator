package testmigrations

func init() { Migrations.MustRegister(nil, noop) }
