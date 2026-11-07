#!/bin/sh
set -eu
research_root=${1:-/var/tmp/guit-cold-plan-analysis}
exec qemu-system-x86_64 \
    -enable-kvm -cpu host -smp 1 -m 2048 -display none -no-reboot \
    -drive "file=$research_root/guest.qcow2,if=virtio,format=qcow2,cache=none" \
    -drive "file=$research_root/seed.iso,media=cdrom,readonly=on" \
    -device virtio-net-pci,netdev=net0 \
    -netdev user,id=net0,hostfwd=tcp:127.0.0.1:18961-:8080 \
    -device virtio-serial-pci \
    -chardev "socket,path=$research_root/agent.sock,server=on,wait=off,id=agent" \
    -device virtserialport,chardev=agent,name=org.qemu.guest_agent.0 \
    -serial "file:$research_root/serial.log" \
    -pidfile "$research_root/qemu.pid" -monitor none
