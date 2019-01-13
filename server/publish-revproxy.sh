#/bin/sh

# rebuild vendor folder
go mod vendor

VERSION=$(date '+%Y-%m-%d-%H-%M-%S')-$(git rev-parse HEAD)
REPO='iad.ocir.io/ssz/vchamber'

docker build -f Dockerfile-revproxy -t $REPO/revproxy:$VERSION .

docker push $REPO/revproxy:$VERSION

kubectl set image deployment/vc-revproxy revproxy=$REPO/revproxy:$VERSION
