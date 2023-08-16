#/bin/bash
rm -rf /home/bot/.tendermint/*
rm -rf contractdb*
rm -rf accdb*
rm -rf condb
rm -rf badg*
rm -rf data
#cp -r /home/userland/tm/* /home/userland/.tendermint/
./tendermint init
#./kvstore init
./kvstore -config ../../../.tendermint/config/config.toml
