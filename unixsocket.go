package endpoint

import (
	"fmt"
	"net"
	"os"
	"syscall"
	"time"
)

//unixsocket实现EndPoint接口
type unixsocket struct {
	fd           int              //套接字文件描述符
	netAddr      *net.UnixAddr    //目标UnixSocket的网络地址
	sockAddr     syscall.Sockaddr //目标UnixSocket的socket地址
	readTimeout  time.Duration    //一次完全数据包的收取超时
	writeTimeout time.Duration    //一次完整数据包的发送超时
}

//创建unixsocket对象
func newUnixSocket() EndPoint {
	return &unixsocket{fd: -1}
}

//建立UnixSocket连接
func (p *unixsocket) Open(config EndPointConfig) (err error) {
	var (
		family int
	)

	c := config.(*UnixSocketConfig)

	//解析目标UnixSocket地址
	if p.sockAddr, family, p.netAddr, err = getUnixSockaddr(c.Network, c.Address); err != nil {
		err = fmt.Errorf("unixsocket: getUnixSockaddr %v %v: %v", c.Network, c.Address)
		return
	}

	//创建客户端套接字
	if p.fd, err = sysSocket(family, syscall.SOCK_STREAM, 0); err != nil {
		err = fmt.Errorf("unixsocket: sysSocket: %v", err)
		return
	}

	//连接UnixSocket地址
	if err = syscall.Connect(p.fd, p.sockAddr); err != nil {
		err = fmt.Errorf("tcp: Connect: %v", os.NewSyscallError("connect", err))
		return
	}

	//设置读写超时
	if c.ReadTimeout > 0 {
		p.readTimeout = c.ReadTimeout
	}
	if c.WriteTimeout > 0 {
		p.writeTimeout = c.WriteTimeout
	}

	return
}

//返回endpoint类型
func (p *unixsocket) Type() EndPointType {
	return EndPointUnix
}

//释放UnixSocket套接字
func (p *unixsocket) Close() error {
	if p.fd != -1 {
		syscall.Close(p.fd)
	}

	return nil
}

//读取UnixSocket数据
func (p *unixsocket) Read(b []byte) (int, error) {
	return syscall.Read(p.fd, b)
}

//写UnixSocket数据
func (p *unixsocket) Write(b []byte) (int, error) {
	return syscall.Write(p.fd, b)
}

//UnixSocket文件句柄
func (p *unixsocket) Fd() int {
	return p.fd
}

//清理UnixSocket的IO缓冲区
func (p *unixsocket) Flush() error {
	return nil
}

//返回UnixSocket网络地址
func (p *unixsocket) NetAddr() net.Addr {
	return p.netAddr
}

//返回UnixSocket的socket地址
func (p *unixsocket) SockAddr() syscall.Sockaddr {
	return p.SockAddr()
}

//返回读超时
func (p *unixsocket) ReadTimeout() time.Duration {
	return p.readTimeout
}

//返回写超时
func (p *unixsocket) WriteTimeout() time.Duration {
	return p.writeTimeout
}

//解析UnixSocket地址
func getUnixSockaddr(proto, addr string) (sa syscall.Sockaddr, family int, unixAddr *net.UnixAddr, err error) {
	unixAddr, err = net.ResolveUnixAddr(proto, addr)
	if err != nil {
		return
	}

	switch unixAddr.Network() {
	case "unix":
		sa, family = &syscall.SockaddrUnix{Name: unixAddr.Name}, syscall.AF_UNIX
	default:
		err = fmt.Errorf("only unix are supported")
	}

	return
}
