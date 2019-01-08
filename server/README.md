# build and run the backend

```
docker build -f Dockerfile-backend -t backend:v1 .
docker run --rm --name test-backend -p 8080:8080 -p 8081:8081 backend:v1
```
