package endpoint

import (
	"fmt"
	"net"
	"syscall"
	"time"
)

//udp实现EndPoint接口
type udp struct {
	fd           int              //套接字文件描述符
	netAddr      *net.UDPAddr     //目标UDP的网络地址
	sockAddr     syscall.Sockaddr //目标UDP的socket地址
	readTimeout  time.Duration    //一次完全数据包的收取超时
	writeTimeout time.Duration    //一次完整数据包的发送超时
}

//创建udp对象
func newUDP() EndPoint {
	return &udp{fd: -1}
}

//初始化UDP套接字
func (p *udp) Open(config EndPointConfig) (err error) {
	var (
		family int
	)

	c := config.(*UDPConfig)

	//解析目标UDP地址
	if p.sockAddr, family, p.netAddr, err = getUDPSockaddr(c.Network, c.Address); err != nil {
		err = fmt.Errorf("udp: getUDPSockaddr %v %v: %v", c.Network, c.Address)
		return
	}

	//创建客户端套接字
	if p.fd, err = sysSocket(family, syscall.SOCK_DGRAM, syscall.IPPROTO_UDP); err != nil {
		err = fmt.Errorf("udp: sysSocket: %v", err)
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
func (p *udp) Type() EndPointType {
	return EndPointUDP
}

//释放UDP套接字
func (p *udp) Close() error {
	if p.fd != -1 {
		syscall.Close(p.fd)
	}

	return nil
}

//读取UDP数据
func (p *udp) Read(b []byte) (n int, err error) {
	n, _, err = syscall.Recvfrom(p.fd, b, 0)
	return
}

//写UDP数据
func (p *udp) Write(b []byte) (int, error) {
	return len(b), syscall.Sendto(p.fd, b, 0, p.sockAddr)
}

//UDP文件句柄
func (p *udp) Fd() int {
	return p.fd
}

//清理UDP的IO缓冲区
func (p *udp) Flush() error {
	return nil
}

//返回UDP网络地址
func (p *udp) NetAddr() net.Addr {
	return p.netAddr
}

//返回UDP的socket地址
func (p *udp) SockAddr() syscall.Sockaddr {
	return p.sockAddr
}

//返回读超时
func (p *udp) ReadTimeout() time.Duration {
	return p.readTimeout
}

//返回写超时
func (p *udp) WriteTimeout() time.Duration {
	return p.writeTimeout
}

//解析UDP地址
func getUDPSockaddr(proto, addr string) (sa syscall.Sockaddr, family int, udpAddr *net.UDPAddr, err error) {
	var udpVersion string

	udpAddr, err = net.ResolveUDPAddr(proto, addr)
	if err != nil {
		return
	}

	udpVersion, err = determineUDPProto(proto, udpAddr)
	if err != nil {
		return
	}

	switch udpVersion {
	case "udp":
		sa, family = &syscall.SockaddrInet4{Port: udpAddr.Port}, syscall.AF_INET
	case "udp4":
		sa4 := &syscall.SockaddrInet4{Port: udpAddr.Port}

		if udpAddr.IP != nil {
			if len(udpAddr.IP) == 16 {
				copy(sa4.Addr[:], udpAddr.IP[12:16]) // copy last 4 bytes of slice to array
			} else {
				copy(sa4.Addr[:], udpAddr.IP) // copy all bytes of slice to array
			}
		}

		sa, family = sa4, syscall.AF_INET
	case "udp6":
		sa6 := &syscall.SockaddrInet6{Port: udpAddr.Port}

		if udpAddr.IP != nil {
			copy(sa6.Addr[:], udpAddr.IP) // copy all bytes of slice to array
		}

		if udpAddr.Zone != "" {
			var iface *net.Interface
			iface, err = net.InterfaceByName(udpAddr.Zone)
			if err != nil {
				return
			}

			sa6.ZoneId = uint32(iface.Index)
		}

		sa, family = sa6, syscall.AF_INET6
	}

	return
}

//判断输入的UDP协议类型是否正确
func determineUDPProto(proto string, addr *net.UDPAddr) (string, error) {
	if addr.IP.To4() != nil {
		return "udp4", nil
	}

	if addr.IP.To16() != nil {
		return "udp6", nil
	}

	switch proto {
	case "udp", "udp4", "udp6":
		return proto, nil
	}

	return "", fmt.Errorf("only udp/udp4/udp6 are supported")
}
