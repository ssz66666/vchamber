#/bin/sh

# rebuild vendor folder
go mod vendor

VERSION=$(date '+%Y-%m-%d-%H-%M-%S')-$(git rev-parse HEAD)
REPO='iad.ocir.io/ssz/vchamber'

docker build -f Dockerfile -t $REPO/frontend:$VERSION .

docker push $REPO/frontend:$VERSION

kubectl set image deployment/vc-frontend frontend=$REPO/frontend:$VERSION
