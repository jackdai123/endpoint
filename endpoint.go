package endpoint

import (
	"fmt"
	"io"
	"net"
	"time"
	"syscall"
)

type EndPointConfig interface{}

//Endpoint类型
type EndPointType int

const (
	EndPointTCP EndPointType = iota
	EndPointUnix
	EndPointUDP
	EndPointSerial
)

//串口和网口的基类
type EndPoint interface {
	io.ReadWriteCloser
	Open(EndPointConfig) error  //打开网口或串口
	Type() EndPointType         //返回Endpoint类型
	Fd() int                    //返回网口或串口的文件句柄
	Flush() error               //清理缓冲区的数据
	NetAddr() net.Addr          //返回网口或串口的网络地址
	SockAddr() syscall.Sockaddr //返回网口或串口的socket地址
}

//打开串口或网口
func Open(c EndPointConfig) (p EndPoint, err error) {
	switch c.(type) {
	case *SerialConfig:
		if _, ok := c.(*SerialConfig); ok {
			p = newSerial()
			err = p.Open(c)
		} else {
			err = fmt.Errorf("type convert to SerialConfig failed")
		}
	case *TCPConfig:
		if _, ok := c.(*TCPConfig); ok {
			p = newTCP()
			err = p.Open(c)
		} else {
			err = fmt.Errorf("type convert to TCPConfig failed")
		}
	case *UDPConfig:
		if _, ok := c.(*UDPConfig); ok {
			p = newUDP()
			err = p.Open(c)
		} else {
			err = fmt.Errorf("type convert to UDPConfig failed")
		}
	case *UnixSocketConfig:
		if _, ok := c.(*UnixSocketConfig); ok {
			p = newUnixSocket()
			err = p.Open(c)
		} else {
			err = fmt.Errorf("type convert to UnixSocketConfig failed")
		}
	default:
		err = fmt.Errorf("wrong type of config")
	}
	return
}

//校验模式
type ParityMode int

const (
	PARITY_NONE  ParityMode = 0 //无校验
	PARITY_ODD   ParityMode = 1 //奇校验
	PARITY_EVEN  ParityMode = 2 //偶校验
	PARITY_MARK  ParityMode = 3 //标记校验（全是1）
	PARITY_SPACE ParityMode = 4 //空白校验（全是0）
)

//串口配置
type SerialConfig struct {
	Address         string        //串口路径，比如/dev/ttyS0
	BaudRate        int           //波特率，默认值9600
	DataBits        int           //数据位长度（5、6、7、8），默认8
	StopBits        int           //停止位长度（1、2），默认1
	Parity          ParityMode    //校验模式
	FirstRecTimeout time.Duration //第一次收到数据的超时
	NextRecTimeout  time.Duration //后续收到数据的间隔超时
	SendTimeout     time.Duration //发送数据的间隔超时
	RS485           RS485Config   //RS485配置
}

//RS485配置
type RS485Config struct {
	Enabled            bool   //开启RS485
	DelayRtsBeforeSend uint32 //发送前RTS延迟（单位毫秒）
	DelayRtsAfterSend  uint32 //发送后RTS延迟（单位毫秒）
	RtsHighDuringSend  bool   //发送期间RTS高电平
	RtsHighAfterSend   bool   //发送后RTS高电平
	RxDuringTx         bool   //支持发送期间读取
}

//TCP socket配置
type TCPSocketOpt int

const (
	TCPDelay TCPSocketOpt = iota
	TCPNoDelay
)

//TCP配置
type TCPConfig struct {
	Network   string        //TCP网络类型（tcp、tcp4、tcp6）
	Address   string        //主机地址，比如192.168.1.1:8080
	KeepAlive time.Duration //TCP保活周期，如果不启用则配0
	NoDelay   TCPSocketOpt  //TCP数据延迟发送，默认no delay
}

//UDP配置
type UDPConfig struct {
	Network string //UDP网络类型（udp、udp4、udp6）
	Address string //主机地址，比如192.168.1.1:8080
}

//UnixSocket配置
type UnixSocketConfig struct {
	Network string //UnixSocket网络类型（unix）
	Address string //UnixSocket文件路径，比如/tmp/a.sock
}
