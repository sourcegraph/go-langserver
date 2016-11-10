#!/bin/sh -x

BIN_YARN=`yarn bin`
BIN_YARN_GLOBAL=`yarn global bin`
export PATH="$BIN_YARN:$BIN_YARN_GLOBAL:$PATH"
