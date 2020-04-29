#!/bin/sh

/root/go/bin/rly l i $chain -f

# rly paths add dawnsworld $chain dawns2$chain

/root/go/bin/rly pth gen dawnsworld transfer $chain transfer dawns2$chain
#    /root/go/bin/rly pth gen $chain transfer dawnsworld transfer $chain2dawns

/root/go/bin/rly tx link dawns2$chain
#rly tx link $chain2dawns

