#!/bin/sh
# Auto-run ActonOS interactive installer & setup wizard on tty1
if [ "$(tty)" = "/dev/tty1" ]; then
    if [ -x /usr/local/bin/acton-install-disk ]; then
        /usr/local/bin/acton-install-disk
    elif [ -x /usr/local/bin/acton-welcome ]; then
        /usr/local/bin/acton-welcome
    fi
fi
