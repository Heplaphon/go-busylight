package main

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"flag"
	"fmt"
	"image/color"
	"log/slog"
	"os"
	"reflect"
	"time"

	"github.com/sstallion/go-hid"
)

// KeepAlive: 8f00000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000ffffff038c
// Red: 1000640000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000ffffff0371

type JmpStep struct {
	OpAndTarget byte
	Repeat      byte
	Red         byte
	Green       byte
	Blue        byte
	OnTime      byte
	OffTime     byte
	Padding     byte
}

type StepB struct {
	Field1 uint64
}

type StepWrapper struct {
	Step interface{} // Holds one of StepA, StepB, StepC, StepD
}

type Packet struct {
	Steps    [7]StepWrapper
	Metadata [8]byte
}

var RED color.RGBA = color.RGBA{0xFF, 0x00, 0x00, 0xFF}

var emptyStep [8]byte = [8]byte{
	0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00,
}

var boot [8]byte = [8]byte{
	0x40, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00,
}

var KeepAlive JmpStep = JmpStep{
	OpAndTarget: 0x8f,
	Repeat:      0x00,
	Red:         0x00,
	Green:       0x00,
	Blue:        0x00,
	OnTime:      0x00,
	OffTime:     0x00,
	Padding:     0x00,
}

// ComputeChecksum calculates the checksum of a byte slice (sum of all bytes)
func ComputeChecksum(data []byte) uint16 {
	var checksum uint16 = 0
	for _, b := range data {
		checksum += uint16(b)
	}
	return checksum
}

// EncodePacketWithChecksum encodes a packet and appends a checksum in the last 2 bytes of metadata
func EncodePacketWithChecksum(packet *Packet) ([]byte, error) {
	// Temporarily zero out the checksum bytes
	packet.Metadata[len(packet.Metadata)-2] = 0
	packet.Metadata[len(packet.Metadata)-1] = 0

	// Encode the packet without checksum
	encodedBytes, err := EncodeStructToBytes(*packet)
	if err != nil {
		return nil, err
	}

	// Compute checksum excluding the last 2 bytes
	checksum := ComputeChecksum(encodedBytes[:len(encodedBytes)-2])

	// Write checksum into the last 2 bytes
	//binary.LittleEndian.PutUint16(encodedBytes[len(encodedBytes)-2:], checksum)
	encodedBytes[len(encodedBytes)-2] = byte(checksum >> 8)   // High byte first
	encodedBytes[len(encodedBytes)-1] = byte(checksum & 0xFF) // Low byte second

	return encodedBytes, nil
}

// EncodeStructToBytes encodes a struct into a byte slice
func EncodeStructToBytes(input interface{}) ([]byte, error) {
	buf := new(bytes.Buffer)
	err := encodeValue(reflect.ValueOf(input), buf)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Recursive function to encode struct fields
func encodeValue(v reflect.Value, buf *bytes.Buffer) error {
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	switch v.Kind() {
	case reflect.Struct:
		// Special handling for StepWrapper
		if v.Type() == reflect.TypeOf(StepWrapper{}) {
			step := v.FieldByName("Step").Interface()
			stepBytes, err := EncodeStructToBytes(step)
			if err != nil {
				return err
			}

			// Validate step is exactly 8 bytes
			if len(stepBytes) != 8 {
				return fmt.Errorf("step struct must be exactly 8 bytes, got %d", len(stepBytes))
			}
			buf.Write(stepBytes)
			return nil
		}

		// Generic struct encoding
		for i := 0; i < v.NumField(); i++ {
			err := encodeValue(v.Field(i), buf)
			if err != nil {
				return err
			}
		}
	case reflect.Array:
		// If it's an array of uint8 ([8]byte for metadata), convert to a slice and write it
		if v.Type().Elem().Kind() == reflect.Uint8 {
			// Convert fixed array to a slice manually
			array := reflect.New(v.Type()).Elem()                   // Create a new instance of the array type
			array.Set(v)                                            // Copy data into the new instance
			buf.Write(array.Slice(0, v.Len()).Interface().([]byte)) // Convert array to slice and wr
		} else {
			// Iterate through array elements and encode each (for arrays like `[7]StepWrapper`)
			for i := 0; i < v.Len(); i++ {
				err := encodeValue(v.Index(i), buf)
				if err != nil {
					return err
				}
			}
		}
	case reflect.Slice:
		// Handle slices normally (just in case we ever use them)
		for i := 0; i < v.Len(); i++ {
			err := encodeValue(v.Index(i), buf)
			if err != nil {
				return err
			}
		}
	case reflect.Uint8, reflect.Int8:
		buf.WriteByte(byte(v.Uint()))
	case reflect.Uint16:
		binary.Write(buf, binary.LittleEndian, uint16(v.Uint()))
	case reflect.Uint32:
		binary.Write(buf, binary.LittleEndian, uint32(v.Uint()))
	case reflect.Uint64:
		binary.Write(buf, binary.LittleEndian, uint64(v.Uint()))
	default:
		return fmt.Errorf("unsupported field type: %s", v.Kind())
	}
	return nil
}

// Clamp returns f clamped to [low, high]
func Clamp(f, low, high int) int {
	if f < low {
		return low
	}
	if f > high {
		return high
	}
	return f
}

var (
	programLevel = new(slog.LevelVar)
	logger       = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: programLevel}))

	red   = flag.Int("red", 0, "Red in light (0-100)")
	green = flag.Int("green", 100, "Green in light (0-100)")
	blue  = flag.Int("blue", 0, "Blue in light (0-100)")
)

func main() {
	flag.Parse()

	var step JmpStep
	step.OpAndTarget = 0x10
	step.Repeat = 0x00
	step.Red = 0x00
	step.Green = 0x64
	step.Blue = 0x08
	step.OnTime = 0x00
	step.OffTime = 0x00
	step.Padding = 0x00

	//redNormalized := ((*red - 0) / (100 - 0)) * 100
	//greenNormalized := ((*green - 0) / (100 - 0)) * 100
	//blueNormalized := ((*blue - 0) / (100 - 0)) * 100
	redNormalized := Clamp(*red, 0, 100)
	greenNormalized := Clamp(*green, 0, 100)
	blueNormalized := Clamp(*blue, 0, 100)

	step.Red = byte(redNormalized)
	step.Green = byte(greenNormalized)
	step.Blue = byte(blueNormalized)

	emptyStep := StepB{
		Field1: 0,
	}

	packet := Packet{
		Steps: [7]StepWrapper{
			{Step: step},
			{Step: emptyStep},
			{Step: emptyStep},
			{Step: emptyStep},
			{Step: emptyStep},
			{Step: emptyStep},
			{Step: emptyStep},
		},
		Metadata: [8]byte{00, 00, 00, 255, 255, 255, 0, 0},
	}

	encodedBytes, err := EncodePacketWithChecksum(&packet)
	if err != nil {
		logger.Error("Error:", err)
		return
	}
	fmt.Println(hex.EncodeToString(encodedBytes))

	dev, err := hid.OpenFirst(0x27bb, 0x3bce)
	if err != nil {
		logger.Error("Error opening device", err)
	}
	defer dev.Close()
	devinfo, err := dev.GetDeviceInfo()
	if err != nil {
		logger.Error("Error opening device", err)
	}
	logger.Info(devinfo.MfrStr + " - " + devinfo.ProductStr)

	written, err := dev.Write(encodedBytes)
	if err != nil {
		logger.Error("Error writing to device ", err)
	}
	logger.Debug("Written to device", "bytes", written)

	keepAliveBytes, err := EncodePacketWithChecksum(&packet)
	if err != nil {
		logger.Error("Error:", err)
		return
	}

	for {
		time.Sleep(5 * time.Second)
		written, err := dev.Write(keepAliveBytes)
		if err != nil {
			logger.Error("Error writing to device ", err)
		}
		logger.Debug("Written to device", "bytes", written)
	}
}
