Use the *clickhouse_database* resource to create a database in a ClickHouse cloud *service*.

Known limitations:

- Changing the comment on a `database` resource is unsupported and will cause the database to be destroyed and recreated. WARNING: you will lose any content of the database if you do so!

