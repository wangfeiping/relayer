#!/bin/sh

#sleep 30  

dateStr=$(date "+%Y-%m-%d %H:%M:%S")

echo $dateStr >> /root/goz.log
/root/go/bin/rly testnets request dawnsworld >> /root/goz.log

