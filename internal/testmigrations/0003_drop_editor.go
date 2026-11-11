package testmigrations

func init() {
	if err := Migrations.Register(noop, nil); err != nil {
		panic(err)
	}
}
