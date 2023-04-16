#/bin/bash
rm -rf /home/userland/.tendermint/*
rm -rf contractdb
rm -rf accdb
rm -rf badg*
#cp -r /home/userland/tm/* /home/userland/.tendermint/
./tendermint init
# ./kvstore init
./kvstore -config ../../../.tendermint/config/config.toml
