#!/bin/sh
# Display ActonOS Welcome diagnostics strictly on interactive user login
case "$-" in
    *i*)
        if [ -t 0 ] && [ -x /usr/local/bin/acton-welcome ]; then
            clear
            /usr/local/bin/acton-welcome
        fi
        ;;
    *)
        ;;
esac
