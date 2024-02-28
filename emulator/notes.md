
export ISRAEL="TRUE" LOG_DATA="FALSE" XDG_RUNTIME_DIR=/run

Run podman system service: `sudo podman system service --time 50`

Open bash in container: `sudo podman exec -it containerID bash`

Show TCP sockets and internal TCP information: `ss -ti`

Remove all podman containers `sudo podman rm --all`

