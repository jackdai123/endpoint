package endpoint

import (
	"fmt"
	"io"
	"net"
	"sync"
	"syscall"
	"time"
)

//Endpoint类型
type EndPointType int

const (
	EndPointTCP EndPointType = iota
	EndPointUnix
	EndPointUDP
	EndPointSerial
)

//EndPoint配置基类
type EndPointConfig interface {
	Type() EndPointType  //返回Endpoint类型
	AddressName() string //以字符串形式返回网口或串口的地址
}

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

//打开串口或网口，并注册到sync.map（key是网络地址、串口文件路径，value是[]EndPoint切片）
func Open(c EndPointConfig, m *sync.Map) (p EndPoint, err error) {
	p = newEndPoint(c)
	if c.Type() == EndPointSerial { //串口
		if _, ok := m.Load(c.AddressName()); ok { // 重复打开串口不可以
			err = fmt.Errorf("opening serial repeated is unsupported")
		} else {
			if err = p.Open(c); err == nil {
				m.Store(c.AddressName(), []EndPoint{p})
			}
		}
	} else { //网口和UnixSocket
		if err = p.Open(c); err == nil {
			if s, ok := m.Load(c.AddressName()); ok { // 网络可以重复打开
				slice := s.([]EndPoint)
				slice = append(slice, p)
				m.Store(c.AddressName(), slice)
			} else {
				m.Store(c.AddressName(), []EndPoint{p})
			}
		}
	}
	return
}

//查找已打开的EndPoint
func Find(c EndPointConfig, m *sync.Map) (slice []EndPoint, find bool) {
	if s, ok := m.Load(c.AddressName()); ok {
		slice = s.([]EndPoint)
		find = true
	} else {
		find = false
	}
	return
}

//删除已打开的EndPoint
func Delete(c EndPointConfig, m *sync.Map) {
	m.Delete(c.AddressName())
}

//初始化EndPoint
func newEndPoint(c EndPointConfig) EndPoint {
	switch c.Type() {
	case EndPointTCP:
		return newTCP()
	case EndPointUDP:
		return newUDP()
	case EndPointUnix:
		return newUnixSocket()
	case EndPointSerial:
		return newSerial()
	default:
		return nil
	}
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

func (c *SerialConfig) Type() EndPointType {
	return EndPointSerial
}

func (c *SerialConfig) AddressName() string {
	return c.Address
}

func (c *TCPConfig) Type() EndPointType {
	return EndPointTCP
}

func (c *TCPConfig) AddressName() string {
	return c.Address
}

func (c *UDPConfig) Type() EndPointType {
	return EndPointUDP
}

func (c *UDPConfig) AddressName() string {
	return c.Address
}

func (c *UnixSocketConfig) Type() EndPointType {
	return EndPointUnix
}

func (c *UnixSocketConfig) AddressName() string {
	return c.Address
}
