# Go Busylight
This is a small project to get kuando busylights to work while trying to minimize external libraries (famous last words, i know)

Currently only tested with and known wokring with the Kuando Busylight UC Alpha. Kuando UC Omega seems to be very similar and should work with just a change of idVendors and idProduct.

## Requirements
- libudev-dev (for debian at least)

## Udev rule
```
KERNEL=="hidraw*", ATTRS{idVendor}=="27bb", ATTRS{idProduct}=="3bce", MODE="0666"
SUBSYSTEM=="usb", ATTRS{idVendor}=="27bb", ATTRS{idProduct}=="3bce", MODE="0666"
```

## TODO
- Make it into a daemon and add a cli part to let it run in the background and be controlled by cli tool (w/ sockets or the like)
- Add sound? ¯\\_(ツ)_/¯
