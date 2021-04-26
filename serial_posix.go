// +build darwin linux freebsd openbsd netbsd

package endpoint

import (
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

//serial实现EndPoint接口
type serial struct {
	fd           int              //串口文件描述符
	address      string           //串口文件路径
	oldTermios   *syscall.Termios //终端配置（波特率、数据位、停止位、校验位等）
	readTimeout  time.Duration    //一次完全数据包的收取超时
	writeTimeout time.Duration    //一次完整数据包的发送超时
}

//RS485相关常量
const (
	rs485Enabled      = 1 << 0
	rs485RTSOnSend    = 1 << 1
	rs485RTSAfterSend = 1 << 2
	rs485RXDuringTX   = 1 << 4
	rs485Tiocs        = 0x542f
)

//RS485驱动配置
type rs485_ioctl_opts struct {
	flags                 uint32
	delay_rts_before_send uint32
	delay_rts_after_send  uint32
	padding               [5]uint32
}

//创建serial
func newSerial() EndPoint {
	return &serial{fd: -1}
}

//打开串口
func (p *serial) Open(config EndPointConfig) (err error) {
	c := config.(*SerialConfig)
	p.address = c.Address

	// See man termios(3).
	// O_NOCTTY: no controlling terminal.
	// O_NDELAY: no data carrier detect.
	p.fd, err = syscall.Open(c.Address, syscall.O_RDWR|syscall.O_NOCTTY|syscall.O_NONBLOCK|syscall.O_CLOEXEC, 0666)
	switch err {
	case nil:
	case syscall.EINTR:
		// Recurse because this is a recoverable error.
		p.Open(config)
		return
	case syscall.ENFILE, syscall.EMFILE:
		err = fmt.Errorf("serial: open serial %v: %v (too many file opened)", c.Address, err)
		return
	default:
		err = fmt.Errorf("serial: open serial %v: %v", c.Address, err)
		return
	}

	termios, err := newTermios(c)
	if err != nil {
		return
	}

	//备份终端配置，使得关闭时可还原配置
	p.backupTermios()
	if err = p.setTermios(termios); err != nil {
		//设置失败，无需还原终端配置
		syscall.Close(p.fd)
		p.fd = -1
		p.oldTermios = nil
		return err
	}

	//设置RS485配置
	if err = enableRS485(p.fd, &c.RS485); err != nil {
		p.Close()
		return err
	}

	//设置读写超时
	if c.ReadTimeout > 0 {
		p.readTimeout = c.ReadTimeout
	} else {
		p.readTimeout = 5000 * time.Millisecond //默认读超时5000ms
	}
	if c.WriteTimeout > 0 {
		p.writeTimeout = c.WriteTimeout
	} else {
		p.writeTimeout = 1000 * time.Millisecond //默认写超时1000ms
	}
	return
}

//返回endpoint类型
func (p *serial) Type() EndPointType {
	return EndPointSerial
}

//关闭串口
func (p *serial) Close() (err error) {
	if p.fd == -1 {
		return
	}
	p.restoreTermios() //还原终端配置
	err = syscall.Close(p.fd)
	p.fd = -1
	p.oldTermios = nil
	return
}

//读取串口，直到所有数据收完或者超时
func (p *serial) Read(b []byte) (n int, err error) {
	var rfds syscall.FdSet
	var readLen, nFd int
	var hasData bool

	fd := p.fd
	expireTime := time.Now().Add(p.readTimeout)

	for { //如遇到EINTR（Interrupted system call）错误，重试
		remainTime := expireTime.Sub(time.Now())
		if remainTime <= 0 { //超时
			err = fmt.Errorf("serial: select timeout: %v", p.readTimeout)
			return
		}

		fdzero(&rfds)
		fdset(fd, &rfds)
		timeout := syscall.NsecToTimeval(remainTime.Nanoseconds()) //设置select超时时间
		nFd, err = syscall.Select(fd+1, &rfds, nil, nil, &timeout)
		if err == nil {
			if nFd == 0 || !fdisset(fd, &rfds) {
				if hasData { //之前读到数据，此处无法判断数据包是否完整，交给上层判断
					return readLen, nil
				} else { //超时
					err = fmt.Errorf("serial: select timeout: %v", p.readTimeout)
					return
				}
			}

			n, err = syscall.Read(fd, b[readLen:])
			if err == nil {
				if n > 0 { //读取数据，继续监听串口，是否还有后续数据
					hasData = true
					readLen += n
				} else { //有IO事件但读不到数据，异常
					err = fmt.Errorf("serial: read no data")
					return
				}
			} else if err != syscall.EINTR { //读失败
				err = fmt.Errorf("serial: could not read: %v", err)
				return
			}
		} else if err != syscall.EINTR { //监听串口失败
			err = fmt.Errorf("serial: could not select: %v", err)
			return
		}
	}
}

//写串口，直到所有数据发完或者超时
func (p *serial) Write(b []byte) (n int, err error) {
	var writeLen, nFd int
	var wfds syscall.FdSet

	expireTime := time.Now().Add(p.writeTimeout)
	bLen := len(b)
	fd := p.fd

	for {
		n, err = syscall.Write(fd, b[writeLen:])
		if err == nil || err == syscall.EINTR {
			writeLen += n
			if writeLen == bLen { //发完数据
				return writeLen, nil
			} else if writeLen > bLen { //发送数据溢出
				err = fmt.Errorf("serial: write overflow: %v > %v", writeLen, bLen)
				return
			}

			for { //没发完数据，等IO可写，继续发送
				remainTime := expireTime.Sub(time.Now())
				if remainTime <= 0 { //超时
					err = fmt.Errorf("serial: select timeout: %v", p.writeTimeout)
					return
				}

				fdzero(&wfds)
				fdset(fd, &wfds)
				timeout := syscall.NsecToTimeval(remainTime.Nanoseconds()) //设置select超时时间
				nFd, err = syscall.Select(fd+1, nil, &wfds, nil, &timeout)
				if err == nil {
					if nFd == 0 || !fdisset(fd, &wfds) { //超时
						err = fmt.Errorf("serial: select timeout: %v", p.writeTimeout)
						return
					}

					break //发送后续数据
				} else if err != syscall.EINTR { //监听串口失败
					err = fmt.Errorf("serial: could not select: %v", err)
					return
				}
			}
		} else { //写失败
			err = fmt.Errorf("serial: could not write: %v", err)
			return
		}
	}
}

//串口文件句柄
func (p *serial) Fd() int {
	return p.fd
}

//清理串口的IO缓冲区
func (p *serial) Flush() error {
	const TCFLSH = 0x540B
	r, _, errno := syscall.Syscall(uintptr(syscall.SYS_IOCTL),
		uintptr(p.fd), uintptr(TCFLSH), uintptr(syscall.TCIOFLUSH))
	if errno != 0 {
		return os.NewSyscallError("SYS_IOCTL (TCFLSH)", errno)
	}
	if r != 0 {
		return errors.New("serial: unknown error from SYS_IOCTL (TCFLSH)")
	}
	return nil
}

//返回串口网络地址
func (p *serial) NetAddr() net.Addr {
	return &net.UnixAddr{
		Net:  "serial",
		Name: p.address,
	}
}

//返回串口的socket地址
func (p *serial) SockAddr() syscall.Sockaddr {
	return nil
}

//返回读超时
func (p *serial) ReadTimeout() time.Duration {
	return p.readTimeout
}

//返回写超时
func (p *serial) WriteTimeout() time.Duration {
	return p.writeTimeout
}

//设置终端配置
func (p *serial) setTermios(termios *syscall.Termios) (err error) {
	if err = tcsetattr(p.fd, termios); err != nil {
		err = fmt.Errorf("serial: could not set setting: %v", err)
	}
	return
}

//备份终端配置
func (p *serial) backupTermios() {
	oldTermios := &syscall.Termios{}
	if err := tcgetattr(p.fd, oldTermios); err != nil {
		// Warning only.
		log.Printf("serial: could not get setting: %v\n", err)
		return
	}
	//关闭时会重新加载
	p.oldTermios = oldTermios
}

//还原终端配置
func (p *serial) restoreTermios() {
	if p.oldTermios == nil {
		return
	}
	if err := tcsetattr(p.fd, p.oldTermios); err != nil {
		// Warning only.
		log.Printf("serial: could not restore setting: %v\n", err)
		return
	}
	p.oldTermios = nil
}

//创建终端配置
func newTermios(c *SerialConfig) (termios *syscall.Termios, err error) {
	var ok bool
	termios = &syscall.Termios{}
	flag := termios.Cflag

	//波特率
	flag, ok = baudRates[c.BaudRate]
	if !ok {
		err = fmt.Errorf("serial: unsupported baud rate %v", c.BaudRate)
		return
	}
	termios.Cflag |= flag

	//输入输出速率
	cfSetIspeed(termios, flag)
	cfSetOspeed(termios, flag)

	//数据位
	flag, ok = charSizes[c.DataBits]
	if !ok {
		err = fmt.Errorf("serial: unsupported character size %v", c.DataBits)
		return
	}
	termios.Cflag &^= syscall.CSIZE
	termios.Cflag |= flag

	//停止位
	switch c.StopBits {
	case 0, 1:
		// Default is one stop bit.
		termios.Cflag &^= syscall.CSTOPB
	case 2:
		// CSTOPB: Set two stop bits.
		termios.Cflag |= syscall.CSTOPB
	default:
		err = fmt.Errorf("serial: unsupported stop bits %v", c.StopBits)
		return
	}

	//校验位
	switch c.Parity {
	case PARITY_NONE:
		termios.Cflag &^= syscall.PARENB
		termios.Iflag &^= syscall.INPCK
	case PARITY_ODD:
		termios.Cflag |= syscall.PARENB
		termios.Cflag |= syscall.PARODD
		termios.Cflag &^= unix.CMSPAR
		termios.Iflag |= syscall.INPCK
	case PARITY_EVEN:
		termios.Cflag |= syscall.PARENB
		termios.Cflag &^= syscall.PARODD
		termios.Cflag &^= unix.CMSPAR
		termios.Iflag |= syscall.INPCK
	case PARITY_MARK:
		termios.Cflag |= syscall.PARENB
		termios.Cflag |= syscall.PARODD
		termios.Cflag |= unix.CMSPAR
		termios.Iflag |= syscall.INPCK
	case PARITY_SPACE:
		termios.Cflag |= syscall.PARENB
		termios.Cflag &^= syscall.PARODD
		termios.Cflag |= unix.CMSPAR
		termios.Iflag |= syscall.INPCK
	default:
		err = fmt.Errorf("serial: unsupported parity %v", c.Parity)
		return
	}

	// Control modes.
	// CREAD: Enable receiver.
	// CLOCAL: Ignore control lines.
	termios.Cflag |= syscall.CREAD | syscall.CLOCAL

	// Special characters.
	// VMIN: Minimum number of characters for noncanonical read.
	// VTIME: Time in deciseconds for noncanonical read.
	// Both are unused as NDELAY is we utilized when opening device.
	return
}

//配置RS485
func enableRS485(fd int, config *RS485Config) error {
	if !config.Enabled {
		return nil
	}

	rs485 := rs485_ioctl_opts{
		rs485Enabled,
		config.DelayRtsBeforeSend,
		config.DelayRtsAfterSend,
		[5]uint32{0, 0, 0, 0, 0},
	}

	if config.RtsHighDuringSend {
		rs485.flags |= rs485RTSOnSend
	}
	if config.RtsHighAfterSend {
		rs485.flags |= rs485RTSAfterSend
	}
	if config.RxDuringTx {
		rs485.flags |= rs485RXDuringTX
	}

	r, _, errno := syscall.Syscall(
		syscall.SYS_IOCTL,
		uintptr(fd),
		uintptr(rs485Tiocs),
		uintptr(unsafe.Pointer(&rs485)))
	if errno != 0 {
		return os.NewSyscallError("SYS_IOCTL (RS485)", errno)
	}
	if r != 0 {
		return errors.New("serial: unknown error from SYS_IOCTL (RS485)")
	}
	return nil
}
