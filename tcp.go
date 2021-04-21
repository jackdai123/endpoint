package endpoint

import (
	"fmt"
	"net"
	"os"
	"syscall"
)

//tcp实现EndPoint接口
type tcp struct {
	fd       int              //套接字文件描述符
	netAddr  *net.TCPAddr     //目标TCP的网络地址
	sockAddr syscall.Sockaddr //目标TCP的socket地址
}

//创建tcp对象
func newTCP() EndPoint {
	return &tcp{fd: -1}
}

//建立TCP连接
func (p *tcp) Open(config EndPointConfig) (err error) {
	var (
		family int
	)

	c := config.(*TCPConfig)

	//解析目标TCP地址
	if p.sockAddr, family, p.netAddr, err = getTCPSockaddr(c.Network, c.Address); err != nil {
		err = fmt.Errorf("tcp: getTCPSockaddr %v %v: %v", c.Network, c.Address)
		return
	}

	//创建客户端套接字
	if p.fd, err = sysSocket(family, syscall.SOCK_STREAM, syscall.IPPROTO_TCP); err != nil {
		err = fmt.Errorf("tcp: sysSocket: %v", err)
		return
	}

	//设置NoDelay和KeepAlive选项
	if err = setNoDelay(p.fd, c.NoDelay); err != nil {
		err = fmt.Errorf("tcp: setNoDelay: %v", err)
		return
	}
	if err = setKeepAlive(p.fd, c.KeepAlive); err != nil {
		err = fmt.Errorf("tcp: setKeepAlive: %v", err)
		return
	}

	//连接TCP地址
	if err = syscall.Connect(p.fd, p.sockAddr); err != nil {
		err = fmt.Errorf("tcp: Connect: %v", os.NewSyscallError("connect", err))
		return
	}

	return
}

//返回endpoint类型
func (p *tcp) Type() EndPointType {
	return EndPointTCP
}

//释放TCP套接字
func (p *tcp) Close() error {
	if p.fd != -1 {
		syscall.Close(p.fd)
	}

	return nil
}

//读取TCP数据
func (p *tcp) Read(b []byte) (int, error) {
	return syscall.Read(p.fd, b)
}

//写TCP数据
func (p *tcp) Write(b []byte) (int, error) {
	return syscall.Write(p.fd, b)
}

//TCP文件句柄
func (p *tcp) Fd() int {
	return p.fd
}

//清理TCP的IO缓冲区
func (p *tcp) Flush() error {
	return nil
}

//返回TCP网络地址
func (p *tcp) NetAddr() net.Addr {
	return p.netAddr
}

//返回TCP的socket地址
func (p *tcp) SockAddr() syscall.Sockaddr {
	return p.sockAddr
}

//解析TCP地址
func getTCPSockaddr(proto, addr string) (sa syscall.Sockaddr, family int, tcpAddr *net.TCPAddr, err error) {
	var tcpVersion string

	tcpAddr, err = net.ResolveTCPAddr(proto, addr)
	if err != nil {
		return
	}

	tcpVersion, err = determineTCPProto(proto, tcpAddr)
	if err != nil {
		return
	}

	switch tcpVersion {
	case "tcp":
		sa, family = &syscall.SockaddrInet4{Port: tcpAddr.Port}, syscall.AF_INET
	case "tcp4":
		sa4 := &syscall.SockaddrInet4{Port: tcpAddr.Port}

		if tcpAddr.IP != nil {
			if len(tcpAddr.IP) == 16 {
				copy(sa4.Addr[:], tcpAddr.IP[12:16]) // copy last 4 bytes of slice to array
			} else {
				copy(sa4.Addr[:], tcpAddr.IP) // copy all bytes of slice to array
			}
		}

		sa, family = sa4, syscall.AF_INET
	case "tcp6":
		sa6 := &syscall.SockaddrInet6{Port: tcpAddr.Port}

		if tcpAddr.IP != nil {
			copy(sa6.Addr[:], tcpAddr.IP) // copy all bytes of slice to array
		}

		if tcpAddr.Zone != "" {
			var iface *net.Interface
			iface, err = net.InterfaceByName(tcpAddr.Zone)
			if err != nil {
				return
			}

			sa6.ZoneId = uint32(iface.Index)
		}

		sa, family = sa6, syscall.AF_INET6
	}

	return
}

//判断输入的TCP协议类型是否正确
func determineTCPProto(proto string, addr *net.TCPAddr) (string, error) {
	if addr.IP.To4() != nil {
		return "tcp4", nil
	}

	if addr.IP.To16() != nil {
		return "tcp6", nil
	}

	switch proto {
	case "tcp", "tcp4", "tcp6":
		return proto, nil
	}

	return "", fmt.Errorf("only tcp/tcp4/tcp6 are supported")
}
