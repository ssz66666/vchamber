#/bin/sh

# rebuild vendor folder
go mod vendor

VERSION=$(date '+%Y-%m-%d-%H-%M-%S')-$(git rev-parse HEAD)
REPO='iad.ocir.io/ssz/vchamber'

docker build -f Dockerfile-backend -t $REPO/backend:$VERSION .

docker push $REPO/backend:$VERSION

kubectl set image deployment/backend websocketbackend=$REPO/backend:$VERSION