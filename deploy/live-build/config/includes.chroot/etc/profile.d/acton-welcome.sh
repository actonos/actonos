#!/bin/sh
# Auto-run ActonOS welcome banner on interactive login on tty1
if [ "$(tty)" = "/dev/tty1" ]; then
    if [ -x /usr/local/bin/acton-welcome ]; then
        /usr/local/bin/acton-welcome
    fi
fi
