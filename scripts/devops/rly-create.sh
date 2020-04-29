#!/bin/sh

GOPATH=/opt/gopath
chainFilesPath=$GOPATH/src/github.com/iqlusioninc/relayer/testnets/relayer-alpha-2
chains=$(ls -l $chainFilesPath |awk '/^-/ {print $NF}')

mnemonic=$MNEM

for chainFile in $chains
do
    chain=${chainFile%%.*}
    echo $chainFile $chain



#    /root/go/bin/rly ch a -f $chainFilesPath/$chain.json

#    /root/go/bin/rly keys restore $chain testkey2 ""

#    /root/go/bin/rly ch edit $chain key testkey2

    /root/go/bin/rly tst request $chain
    
done




