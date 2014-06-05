package main

import (
    "bufio"
    "encoding/json"
    "errors"
    "flag"
    "fmt"
    "github.com/amir/raidman"
    "github.com/huin/goserial"
    "io"
    "log"
    "net/url"
    "os"
    "strconv"
    "strings"
    "time"
)

var serialPort string
var serialBaud int
var riemannProto string
var riemannHost string
var riemannPort int
var eventTtl float32
var eventHost string

var riemannClient *raidman.Client

func init() {
    const (
        defaultSerialPort   = "/dev/ttyUSB0"
        defaultSerialBaud   = 19200
        defaultRiemannProto = "tcp"
        defaultRiemannHost  = "riemann"
        defaultRiemannPort  = 5555
        defaultEventTtl     = 15 // typically receive events every 10 seconds
                                 // extra 5s allows for network or other processing delays
        defaultEventHost = "main electrical panel"
    )
    flag.StringVar(&serialPort, "serial-port", defaultSerialPort, "")
    flag.IntVar(&serialBaud, "serial-baud", defaultSerialBaud, "")
    flag.StringVar(&riemannProto, "riemann-protocol", defaultRiemannProto, "")
    flag.StringVar(&riemannHost, "riemann-host", defaultRiemannHost, "")
    flag.IntVar(&riemannPort, "riemann-port", defaultRiemannPort, "")
    eventTtl = float32(*flag.Int("ttl", defaultEventTtl, ""))
    flag.StringVar(&eventHost, "host", defaultEventHost, "")
}

func extractSecondsCounter(vals url.Values) (int, error) {
    return strconv.Atoi(vals.Get("SC"))
}

func extractSerialNumber(vals url.Values) (string, error) {
    serialNo := vals.Get("SN")
    if len(serialNo) < 1 {
        return serialNo, errors.New("No serial number found under 'SN' key.")
    }
    return serialNo, nil
}

func extractVolts(vals url.Values) (float64, error) {
    voltsTimes10, err := strconv.ParseFloat(vals.Get("V"), 64)
    return voltsTimes10 / 10, err
}

func extractWattSecondCount(vals url.Values, chNo int) (int, error) {
    return strconv.Atoi(vals.Get(fmt.Sprintf("c%d", chNo)))
}

func extractCsvFloats(vals url.Values, key string, count int) ([]float64, error) {
    strs := strings.SplitN(vals.Get(key), ",", count)
    nums := make([]float64, count)
    var err error
    for i := 1; i <= count; i++ {
        nums[i-1], err = strconv.ParseFloat(strs[i-1], 64)
        if err != nil {
            return nums, err
        }
    }
    return nums, nil
}

func extractCsvInts(vals url.Values, key string, count int) ([]int, error) {
    strs := strings.SplitN(vals.Get(key), ",", count)
    nums := make([]int, count)
    var err error
    for i := 1; i <= count; i++ {
        nums[i-1], err = strconv.Atoi(strs[i-1])
        if err != nil {
            return nums, err
        }
    }
    return nums, nil
}

func eventsFromMap(vals url.Values) ([]raidman.Event, error) {
    seconds, _ := extractSecondsCounter(vals)
    serialNo, _ := extractSerialNumber(vals)
    volts, _ := extractVolts(vals)
    events := make([]raidman.Event, 256)
    eventCount := 0

    attrs := map[string]string{
        "serial_number": serialNo,
        "seconds":       fmt.Sprintf("%d", seconds),
        "volts":         fmt.Sprintf("%g", volts),
    }

    // watt-second channels
    for i := 1; i <= 48; i++ {
        service := fmt.Sprintf("ch%02d", i)
        wattSec, _ := extractWattSecondCount(vals, i)
        events[eventCount] = raidman.Event{
            State:      "ok",
            Host:       eventHost,
            Service:    service,
            Metric:     wattSec,
            Ttl:        eventTtl,
            Attributes: attrs,
        }
        eventCount++
    }

    // temperature channels
    temps, _ := extractCsvFloats(vals, "T", 8)
    for i := range temps {
        service := fmt.Sprintf("temp%02d", i+1)
        events[eventCount] = raidman.Event{
            State:      "ok",
            Host:       eventHost,
            Service:    service,
            Metric:     temps[i],
            Ttl:        eventTtl,
            Attributes: attrs,
        }
        eventCount++
    }

    // pulse channels
    pulses, _ := extractCsvInts(vals, "PL", 4)
    for i := range pulses {
        service := fmt.Sprintf("pulse%02d", i+1)
        events[eventCount] = raidman.Event{
            State:      "ok",
            Host:       eventHost,
            Service:    service,
            Metric:     pulses[i],
            Ttl:        eventTtl,
            Attributes: attrs,
        }
        eventCount++
    }

    return events[0:eventCount], nil
}

func sendToRiemann(vals url.Values) (*raidman.Event, error) {
    events, err := eventsFromMap(vals)
    if err != nil {
        return nil, err
    }

    for i := range events {
        if err := riemannClient.Send(&events[i]); err != nil {
            return &events[i], err
        }
    }

    return nil, nil
}

func printAsJson(vals url.Values) error {
    var err error
    var jsonMap map[string]interface{}
    jsonMap = make(map[string]interface{})

    seconds, _ := extractSecondsCounter(vals)
    serialNo, _ := extractSerialNumber(vals)
    volts, _ := extractVolts(vals)

    jsonMap["timestamp"] = time.Now().Format(time.RFC3339)
    jsonMap["seconds"] = seconds
    jsonMap["serial_number"] = serialNo
    jsonMap["volts"] = volts

    // watt-second channels
    for i := 1; i <= 48; i++ {
        wattSec, _ := extractWattSecondCount(vals, i)
        jsonMap[fmt.Sprintf("ch%02d", i)] = wattSec
    }

    // temperature channels
    temps, _ := extractCsvFloats(vals, "T", 8)
    for i := range temps {
        jsonMap[fmt.Sprintf("temp%02d", i+1)] = temps[i]
    }

    // pulse channels
    pulses, _ := extractCsvInts(vals, "PL", 4)
    for i := range pulses {
        jsonMap[fmt.Sprintf("pulse%02d", i+1)] = pulses[i]
    }

    bytes, err := json.Marshal(jsonMap)
    if err != nil {
        return err
    }

    _, err = os.Stdout.Write(bytes)
    if err != nil {
        return err
    }

    _, err = os.Stdout.WriteString("\n")
    if err != nil {
        return err
    }

    return nil
}

func openSerialPortOrExit() io.ReadWriteCloser {
    c := &goserial.Config{Name: serialPort, Baud: serialBaud}
    ser, err := goserial.OpenPort(c)
    if err != nil {
        log.Printf("Could not connect to serial port %s.", serialPort)
        log.Fatal(err)
    }
    return ser
}

func connectToRiemannOrExit() *raidman.Client {
    socket := fmt.Sprintf("%s:%d", riemannHost, riemannPort)
    client, err := raidman.Dial(riemannProto, socket)
    if err != nil {
        log.Printf("Could not connect to riemann at %s.", socket)
        log.Fatal(err)
    }
    return client
}

func main() {
    flag.Parse()

    ser := openSerialPortOrExit()
    defer ser.Close()

    riemannClient = connectToRiemannOrExit()
    defer riemannClient.Close()

    scanner := bufio.NewScanner(bufio.NewReader(ser))
    for scanner.Scan() {
        line := scanner.Text()
        if strings.HasPrefix(line, "GET ") {
            uri := strings.Fields(line)[1]
            u, err := url.ParseRequestURI(uri)
            if err != nil {
                log.Printf("Failed to parse URI: %v, %v", uri, err)
                continue
            }

            vals := u.Query()
            printAsJson(vals)
            if evt, err := sendToRiemann(vals); err != nil {
                log.Printf("Failed to send event (%v) to riemann: %v", evt, err)
            }
        }
    }
}
