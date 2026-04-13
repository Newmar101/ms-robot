package streaming

import (
	"fmt"
	"io"
	"log"
	"net"
)

// ADBSocket ADB Socket连接
type ADBSocket struct {
	conn            net.Conn
	closed          bool
	deviceInfoRead  bool
	videoHeaderRead bool // 是否已读取视频头（codec ID + width + height）
	configRead      bool
	frameCount      uint64 // 已接收视频帧计数（用于调试日志）
	lastPTS         uint64 // 上一个帧的PTS
	lastIsKeyFrame  bool   // 上一个帧是否为关键帧
	lastIsConfig    bool   // 上一个帧是否为配置帧
}

// ReadRaw 从底层连接读取 n 字节（用于恢复流时消费新 header 等）
func (s *ADBSocket) ReadRaw(n int) ([]byte, error) {
	if s.closed {
		return nil, io.EOF
	}
	buf := make([]byte, n)
	_, err := io.ReadFull(s.conn, buf)
	return buf, err
}

// Read 读取数据
// 按照 test_scrcpy_stream.go 的方式，直接读取，不做额外的检测操作
func (s *ADBSocket) Read(p []byte) (n int, err error) {
	if s.closed {
		return 0, io.EOF
	}
	// 直接读取，不做额外的检测，避免消耗数据或影响连接
	return s.conn.Read(p)
}

// Write 写入数据
func (s *ADBSocket) Write(p []byte) (n int, err error) {
	if s.closed {
		return 0, fmt.Errorf("socket已关闭")
	}
	return s.conn.Write(p)
}

// Close 关闭连接
func (s *ADBSocket) Close() error {
	if s.closed {
		return nil
	}
	s.closed = true
	if err := s.conn.Close(); err != nil {
		log.Printf("[ADBSocket] Close: 关闭连接时出错: %v", err)
		return err
	}
	return nil
}

// ReadH264Stream 读取H.264视频流
// scrcpy协议格式（根据 Streamer.java 和 demuxer.c）：
// 1. 设备信息（64字节，可选，如果 sendDeviceMeta=true）
// 2. 视频头（12字节，可选，如果 sendCodecMeta=true）：
//   - codec ID (4字节, big-endian)
//   - width (4字节, big-endian)
//   - height (4字节, big-endian)
//
// 3. 数据包（循环，如果 sendFrameMeta=true）：
//   - PTS + 标志位 (8字节, big-endian)
//   - bit 63: PACKET_FLAG_CONFIG (配置包)
//   - bit 62: PACKET_FLAG_KEY_FRAME (关键帧)
//   - 低62位: PTS (时间戳，微秒)
//   - 数据大小 (4字节, big-endian)
//   - 数据内容 (N字节)
func (s *ADBSocket) ReadH264Stream() ([]byte, error) {
	// 首次读取设备信息（64字节，可选）
	if !s.deviceInfoRead {
		// 按照 test_scrcpy_stream.go，直接读取设备信息（64字节），没有 dummy byte
		// 按照 test_scrcpy_stream.go，使用 io.ReadFull 直接读取64字节
		deviceInfo := make([]byte, 64)
		if _, err := io.ReadFull(s, deviceInfo); err != nil {
			return nil, fmt.Errorf("读取设备信息失败: %v", err)
		}

		s.deviceInfoRead = true
		// 解析设备名称（UTF-8，以 \0 结尾）
		deviceName := string(deviceInfo)
		if idx := len(deviceName); idx > 0 {
			if nullIdx := 0; nullIdx < len(deviceInfo) && deviceInfo[nullIdx] == 0 {
				// 找到第一个 \0
				for i := 0; i < len(deviceInfo); i++ {
					if deviceInfo[i] == 0 {
						nullIdx = i
						break
					}
				}
				deviceName = string(deviceInfo[:nullIdx])
			}
		}
		_ = deviceName
	}

	// 首次读取视频头（12字节，如果 sendCodecMeta=true）
	// 根据 scrcpy 源码，默认 sendCodecMeta=true，所以会发送视频头
	if !s.videoHeaderRead {
		videoHeader := make([]byte, 12)
		if _, err := io.ReadFull(s, videoHeader); err != nil {
			return nil, fmt.Errorf("读取视频头失败: %v", err)
		}

		// 解析视频头（big-endian）
		codecID := uint32(videoHeader[0])<<24 | uint32(videoHeader[1])<<16 |
			uint32(videoHeader[2])<<8 | uint32(videoHeader[3])
		width := uint32(videoHeader[4])<<24 | uint32(videoHeader[5])<<16 |
			uint32(videoHeader[6])<<8 | uint32(videoHeader[7])
		height := uint32(videoHeader[8])<<24 | uint32(videoHeader[9])<<16 |
			uint32(videoHeader[10])<<8 | uint32(videoHeader[11])

		s.videoHeaderRead = true
		_, _ = width, height

		// 验证 codec ID（H.264 = 0x68323634 = "h264" in ASCII）
		if codecID != 0x68323634 {
			log.Printf("警告: 意外的 codec ID: 0x%08x (期望 H.264: 0x68323634)", codecID)
		}
	}

	// 注意：根据 scrcpy 源码，H.264 配置（SPS/PPS）不是单独发送的
	// 而是作为普通数据包发送，通过 PTS 的标志位（bit 63）来标识
	// 所以不需要单独读取配置，配置会在第一个数据包中收到
	if !s.configRead {
		s.configRead = true
	}

	// 读取H.264帧数据
	// scrcpy帧格式（根据 Streamer.java 和 demuxer.c）：
	// - [8字节] PTS + 标志位（big-endian，Java ByteBuffer 默认）
	//   - bit 63: PACKET_FLAG_CONFIG (配置包)
	//   - bit 62: PACKET_FLAG_KEY_FRAME (关键帧)
	//   - 低62位: PTS (时间戳，微秒)
	// - [4字节] 数据大小（big-endian）
	// - [N字节] 数据内容
	ptsAndFlags := make([]byte, 8)
	var n int
	var err error
	n, err = io.ReadFull(s, ptsAndFlags)
	if err != nil {
		// 如果是 EOF 且读取了0字节，可能是对端暂时没发数据，连接还在
		// 这种情况下不应该频繁报错，让上层安静等待
		if err == io.EOF && n == 0 {
			// 检查连接是否真的关闭了
			if s.closed {
				return nil, io.EOF
			}
			// 连接还在，只是暂时没数据，返回特殊错误让上层知道这是"暂时没数据"
			return nil, fmt.Errorf("暂时无数据")
		}
		// 其他错误（包括部分读取失败）才记录日志
		if n > 0 {
			log.Printf("读取时间戳+标志位部分失败: 已读取 %d/8 字节, 内容(hex): %x, 错误: %v", n, ptsAndFlags[:n], err)
		}
		return nil, fmt.Errorf("读取时间戳+标志位失败: %v", err)
	}

	// 解析时间戳和标志位（big-endian，与 scrcpy 客户端一致）
	ptsValue := uint64(ptsAndFlags[0])<<56 | uint64(ptsAndFlags[1])<<48 |
		uint64(ptsAndFlags[2])<<40 | uint64(ptsAndFlags[3])<<32 |
		uint64(ptsAndFlags[4])<<24 | uint64(ptsAndFlags[5])<<16 |
		uint64(ptsAndFlags[6])<<8 | uint64(ptsAndFlags[7])

	// 根据 scrcpy 源码 (Streamer.java 和 demuxer.c)：
	// - bit 63: PACKET_FLAG_CONFIG (配置包)
	// - bit 62: PACKET_FLAG_KEY_FRAME (关键帧)
	// - 低62位: PTS (时间戳，微秒)
	// 注意：如果是配置包，PTS 部分为 0，只有标志位
	isConfig := (ptsValue & (1 << 63)) != 0   // bit 63: 配置包
	isKeyFrame := (ptsValue & (1 << 62)) != 0 // bit 62: 关键帧
	// 使用与 scrcpy demuxer.c 相同的掩码：SC_PACKET_PTS_MASK = (SC_PACKET_FLAG_KEY_FRAME - 1)
	pts := ptsValue & ((1 << 62) - 1) // 低62位: 时间戳

	frameSize := make([]byte, 4)
	n, err = io.ReadFull(s, frameSize)
	if err != nil {
		if n > 0 {
			log.Printf("读取帧大小部分失败: 已读取 %d/4 字节, 内容(hex): %x, 错误: %v", n, frameSize[:n], err)
		} else {
			log.Printf("读取帧大小失败: 未读取任何数据, 错误: %v", err)
		}
		return nil, fmt.Errorf("读取帧大小失败: %v", err)
	}

	// 解析帧大小（big-endian）
	frameLen := int(frameSize[0])<<24 | int(frameSize[1])<<16 |
		int(frameSize[2])<<8 | int(frameSize[3])

	if frameLen == 0 {
		return nil, nil // 空帧
	}

	if frameLen > 10*1024*1024 { // 10MB限制
		return nil, fmt.Errorf("帧大小异常: %d字节", frameLen)
	}

	frame := make([]byte, frameLen)
	n, err = io.ReadFull(s, frame)
	if err != nil {
		if n > 0 {
			previewBytes := min(32, n)
			log.Printf("读取帧数据部分失败: 已读取 %d/%d 字节, 前%d字节(hex): %x, 错误: %v", n, frameLen, previewBytes, frame[:previewBytes], err)
			// 如果已经读取了部分数据，返回这部分数据，让调用者决定如何处理
			// 这可能是一帧不完整的数据
			return frame[:n], fmt.Errorf("读取帧数据不完整: 已读取 %d/%d 字节, 错误: %v", n, frameLen, err)
		} else {
			log.Printf("读取帧数据失败: 未读取任何数据, 帧大小: %d, 错误: %v", frameLen, err)
		}
		return nil, fmt.Errorf("读取帧数据失败: %v (帧大小: %d, 已读取: %d)", err, frameLen, n)
	}

	// 帧已读取（不打印详细日志，避免刷屏）

	// 将PTS信息存储到ADBSocket中，供后续使用
	s.frameCount++
	s.lastPTS = pts
	s.lastIsKeyFrame = isKeyFrame
	s.lastIsConfig = isConfig

	return frame, nil
}
