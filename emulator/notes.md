
export ISRAEL="TRUE" LOG_DATA="FALSE" XDG_RUNTIME_DIR=/run

Run podman system service: `sudo podman system service --time 50`

Open bash in container: `sudo podman exec -it containerIP bash`

Show TCP sockets and internal TCP information: `ss -ti`

Remove all podman containers `sudo podman rm --all`

iperf3 server `iperf3 -s -p 7575`

iperf client `iperf3 -c serverIP -p 7575`

Copy file from container `sudo podman cp containerName:filePathInContainer filePathOnMyPc`




Image location `/home/simon/.local/share/containers/storage/overlay-images/` mby?

See information similar to what is in the Dockerfile wrt an groundstation image: `podman history groundstation --no-trunc`
^ Better information when going to the docker repo

Create new docker image `https://stackoverflow.com/questions/52415858/updating-a-docker-image-without-the-original-dockerfile`

Make statically linked binary of tcp_metrics: `CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo ./cmd/tcp_metrics/main.go`
^ On dockerizing an application: `https://www.cloudbees.com/blog/building-minimal-docker-containers-for-go-applications#part-2-dockerize`

Build groundstation image from Dockerfile including the tcp_metrics binary which is in the same directory `docker build -t groundstation:1.0 ~/Documents/repositories/P8-project/satellite_tcp_emulator/`

# Push new image to docker hub

1. Login specifying the exact repository I want to access

General : `podman login -u username -p password docker.io/username/dockerrepo`
Example : `podman login -u sjuhl1 -p password docker.io/sjuhl1/groundstation`

2. Tag image (assign new name to image)

General : `podman tag localimagename:latest username/dockerrepo`
Example : `podman tag groundstation:latest sjuhl1/groundstation`

^ In the above example I have an image with the repository name `localhost/groundstation`. It is possible to have several images with that repo name but different tags assigned (e.g.: `v1.X.X` or `latest`)

3. Push image to docker repo

General : `podman push username/dockerrepo`
Example : `podman push sjuhl1/groundstation`

# Error messages and fixes

* `2024/03/28 09:38:45 rtt106 1unsupproted kv int slash pair`

* If unable to access register: `sudo sysctl -w net.ipv4.tcp_mtu_probing=1` 

