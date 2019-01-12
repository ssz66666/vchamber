#/bin/sh

# rebuild vendor folder
go mod vendor

VERSION=$(date '+%Y-%m-%d-%H-%M-%S')-$(git rev-parse HEAD)
REPO='iad.ocir.io/ssz/vchamber'

docker build -f Dockerfile-orchestrator -t $REPO/orchestrator:$VERSION .

docker push $REPO/orchestrator:$VERSION

kubectl set image deployment/vc-orchestrator orchestrator=$REPO/orchestrator:$VERSION
