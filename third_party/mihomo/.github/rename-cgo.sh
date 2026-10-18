#!/bin/bash

FILENAMES=$(ls)
for FILENAME in $FILENAMES
do
    if [[ $FILENAME =~ "darwin-10.16-arm64" ]];then
        echo "rename darwin-10.16-arm64 $FILENAME"
        mv $FILENAME citrus-darwin-arm64-cgo
    elif [[ $FILENAME =~ "darwin-10.16-amd64" ]];then
        echo "rename darwin-10.16-amd64 $FILENAME"
        mv $FILENAME citrus-darwin-amd64-cgo
    elif [[ $FILENAME =~ "windows-4.0-386" ]];then
        echo "rename windows 386 $FILENAME"
        mv $FILENAME citrus-windows-386-cgo.exe
    elif [[ $FILENAME =~ "windows-4.0-amd64" ]];then
        echo "rename windows amd64 $FILENAME"
        mv $FILENAME citrus-windows-amd64-cgo.exe
    elif [[ $FILENAME =~ "citrus-linux-arm-5" ]];then
        echo "rename citrus-linux-arm-5 $FILENAME"
        mv $FILENAME citrus-linux-armv5-cgo
    elif [[ $FILENAME =~ "citrus-linux-arm-6" ]];then
        echo "rename citrus-linux-arm-6 $FILENAME"
        mv $FILENAME citrus-linux-armv6-cgo
    elif [[ $FILENAME =~ "citrus-linux-arm-7" ]];then
        echo "rename citrus-linux-arm-7 $FILENAME"
        mv $FILENAME citrus-linux-armv7-cgo
    elif [[ $FILENAME =~ "linux" ]];then
        echo "rename linux $FILENAME"
        mv $FILENAME $FILENAME-cgo
    elif [[ $FILENAME =~ "android" ]];then
        echo "rename android $FILENAME"
        mv $FILENAME $FILENAME-cgo
    else echo "skip $FILENAME"
    fi
done