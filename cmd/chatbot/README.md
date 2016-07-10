Dumb probabilistic Markov model chatbot.

Usage help with `-h`. Config file format is JSON mirroring config struct type in source code.

#### Database

Assumes Postgres installed and running on default port (5432) with schema:

```sql
CREATE SCHEMA markovbot;

CREATE TABLE markovbot.graph (
	a text NOT NULL,
	b text NOT NULL,
	c integer NOT NULL,
	UNIQUE (a, b)
);

CREATE TABLE markovbot.first_words (
	word text UNIQUE NOT NULL,
	n integer NOT NULL DEFAULT 1
);
```

#### Env

* `GAS_DB_NAME`: value should be `postgres`
* `GAS_DB_PARAMS`: postgres connection string e.g. `dbname=... user=... password=... sslmode=...`
