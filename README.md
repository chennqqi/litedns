# litedns

- upstream
- cache with expire
- redis as backend

## api

```
GET /resolve/:name
200
Content-Type: application/json
{
	"name": ""
	"A":[
	],
	"AAAA": [
	],
	"expire" : "240s"
	"ttl" : "10s"
}

SET /resolve/:name
Content-Type: application/json
{
	"name": "name"
	"A":[
	],
	"AAAA": [
	]
	"expire" : "120s"
	"ttl" : "10s"
}
200
```

## TODO:
1. support `GET /resolve` by upstream
2. compatible with httpdns