# How to build and run the front end

navigate the the client folder (this folder), then

```
docker build -t frontend:v1 .
docker run -p 80:80 --rm --name test-nginx frontend:v1
```