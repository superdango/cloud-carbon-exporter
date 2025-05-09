# Primitives Bootstrap

## Processors TDP

Generate `processors.csv`

```
$ docker run  -p 5000:5000 ghcr.io/boavizta/boaviztapi:latest uvicorn boaviztapi.main:app --host 0.0.0.0 --port 5000 --workers 4
$ go run generate_processors.go
$ ./merge_processors.sh
```

