#/bin/sh

# rebuild vendor folder
go mod vendor

VERSION=$(date '+%Y-%m-%d-%H-%M-%S')-$(git rev-parse HEAD)
REPO='iad.ocir.io/ssz/vchamber'

docker build -f Dockerfile-backend -t $REPO/backend:$VERSION .
docker build -f Dockerfile-revproxy -t $REPO/revproxy:$VERSION .
docker build -f Dockerfile-scheduler -t $REPO/scheduler:$VERSION .
docker build -f Dockerfile-orchestrator -t $REPO/orchestrator:$VERSION .

docker push $REPO/backend:$VERSION
docker push $REPO/revproxy:$VERSION
docker push $REPO/scheduler:$VERSION
docker push $REPO/orchestrator:$VERSION

kubectl set image statefulset/vc-backend wsbackend=$REPO/backend:$VERSION
kubectl set image deployment/vc-revproxy revproxy=$REPO/revproxy:$VERSION
kubectl set image deployment/vc-scheduler scheduler=$REPO/scheduler:$VERSION
kubectl set image deployment/vc-orchestrator orchestrator=$REPO/orchestrator:$VERSION
