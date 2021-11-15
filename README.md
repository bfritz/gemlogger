Quick hack to pipe energy readings from [GreenEye Monitor](https://www.brultech.com/greeneye/) (GEM) to [Riemann](http://riemann.io/) for further processing.

GEM is configured to send readings every 10 seconds encoded as GET requests, e.g.:

```
GET /monitor/gem/?SN=01000123&SC=13399560&V=1202&c1=130106211517&c2=4369428737&c3=8569665918&c4=2298732&c5=14367144577&c6=7880425504&c7=2115745526&c8=25808141664&c9=6817404343&c10=62697109&c11=2529663042&c12=2309619&c13=2302250&c14=2311662&c15=2305541&c16=2324795&c17=1243557270&c18=1619652535&c19=524654949&c20=2115845649&c21=2424805589&c22=15285944150&c23=2103980591&c24=31266216421&c25=2335442&c26=2331366&c27=238402745&c28=1749214052&c29=2330114&c30=2343316&c31=2331249&c32=2338507&c33=0&c34=0&c35=0&c36=0&c37=0&c38=0&c39=0&c40=0&c41=0&c42=0&c43=0&c44=0&c45=0&c46=0&c47=0&c48=0&PL=0,0,0,0&T=,,,,,,,&Resp= HTTP/1.0
Host: 192.168.5.5

```

Xbee radio connected to server echos the readings on `/dev/ttyUSB0`.

`gemlogger` listens to `/dev/ttyUSB0`, parses the URLs, logs the data
as JSON on stdout, and forwards to Riemann for processing.  The JSON
output is easy to ingest with Logstash and useful for archiving.

### Usage:

```sh
# inside a tmux session
while true; do
    ./gemlogger --riemann-host=172.22.4.123 --ttl=15 | tee -a ~/logs/gem.log
    sleep 2
    echo restarting
done
```

### Disclaimer:

This is my first project written in [Go](https://golang.org/).  The code is
almost certainly not idiomatic.  There is a fair chance it will eat small
children, burn down your house, or both.
