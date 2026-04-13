// scrcpy 控制命令工具
// 所有控制消息类型定义
const ControlMessageType = {
    INJECT_KEYCODE: 0x00,
    INJECT_TEXT: 0x01,
    INJECT_TOUCH_EVENT: 0x02,
    INJECT_SCROLL_EVENT: 0x03,
    BACK_OR_SCREEN_ON: 0x04,
    EXPAND_NOTIFICATION_PANEL: 0x05,
    EXPAND_SETTINGS_PANEL: 0x06,
    COLLAPSE_PANELS: 0x07,
    GET_CLIPBOARD: 0x08,
    SET_CLIPBOARD: 0x09,
    SET_DISPLAY_POWER: 0x0A,
    ROTATE_DEVICE: 0x0B,
    UHID_CREATE: 0x0C,
    UHID_INPUT: 0x0D,
    UHID_DESTROY: 0x0E,
    OPEN_HARD_KEYBOARD_SETTINGS: 0x0F,
    START_APP: 0x10,
    RESET_VIDEO: 0x11
};

// 与 scrcpy ControlMessage.COPY_KEY_* / app/src/input_manager.c get_device_clipboard 一致。
// clipboard_autosync 开启时服务端不会主动读剪贴板，须用 COPY（或 CUT）注入按键后由监听推送到前端。
const ClipboardCopyKey = {
    NONE: 0,
    COPY: 1,
    CUT: 2
};

// Android KeyEvent 键码
const KeyCode = {
    BACK: 4,
    HOME: 3,
    MENU: 82,
    APP_SWITCH: 187, // 最近任务键
    VOLUME_UP: 24,
    VOLUME_DOWN: 25,
    VOLUME_MUTE: 164, // 静音键
    POWER: 26,
    ENTER: 66,
    DEL: 67,
    SPACE: 62,
    TAB: 61
};

// MotionEvent 动作
const MotionAction = {
    DOWN: 0,
    UP: 1,
    MOVE: 2,
    CANCEL: 3,
    POINTER_DOWN: 5,  // 多点触控：其他手指按下
    POINTER_UP: 6     // 多点触控：其他手指抬起
};

// 工具函数：将数字转换为 big-endian 字节数组（支持负数，64位有符号整数）
function toBigEndianBytes(value, byteLength) {
    const bytes = new Uint8Array(byteLength);
    // 将 value 转换为 BigInt 以支持 64 位有符号整数
    let bigValue = BigInt(value);
    
    // 如果是负数，转换为补码形式
    if (bigValue < 0) {
        // 计算补码：2^(byteLength*8) + value
        const maxValue = BigInt(1) << BigInt(byteLength * 8);
        bigValue = maxValue + bigValue;
    }
    
    // 转换为字节数组
    for (let i = byteLength - 1; i >= 0; i--) {
        bytes[i] = Number(bigValue & BigInt(0xFF));
        bigValue = bigValue >> BigInt(8);
    }
    
    return bytes;
}

// 工具函数：将数字编码为 VarInt（变长整数，scrcpy 使用）
// VarInt 编码：每个字节的最高位（bit 7）表示是否还有后续字节（1=有，0=无）
// 低7位是实际数据的一部分
function encodeVarInt(value) {
    const bytes = [];
    while (value >= 0x80) {
        bytes.push((value & 0x7F) | 0x80); // 最高位设为1，表示还有后续字节
        value = value >>> 7;
    }
    bytes.push(value & 0x7F); // 最后一个字节，最高位为0
    return new Uint8Array(bytes);
}

// 工具函数：将字符串转换为 UTF-8 字节数组
function stringToUtf8Bytes(str) {
    return new Uint8Array(new TextEncoder().encode(str));
}

// 工具函数：构建控制消息
function buildControlMessage(type, data = []) {
    return new Uint8Array([type, ...data]);
}

// 1. 注入按键事件
function injectKeycode(deviceUDID, keycode, action = MotionAction.DOWN) {
    const repeat = 0;
    const metastate = 0;
    const data = new Uint8Array([
        action,
        ...toBigEndianBytes(keycode, 4),
        ...toBigEndianBytes(repeat, 4),
        ...toBigEndianBytes(metastate, 4)
    ]);
    return sendControlMessage(deviceUDID, ControlMessageType.INJECT_KEYCODE, data);
}

// 2. 注入文本
function injectText(deviceUDID, text) {
    const textBytes = stringToUtf8Bytes(text);
    // scrcpy使用4字节big-endian长度（不是2字节！）
    // 参考 ControlMessageReader.java: parseInjectText() -> parseString() -> parseString(4)
    const lengthBytes = toBigEndianBytes(textBytes.length, 4);
    const data = new Uint8Array([...lengthBytes, ...textBytes]);
    return sendControlMessage(deviceUDID, ControlMessageType.INJECT_TEXT, data);
}

// 获取设备屏幕尺寸（从video元素）
function getDeviceScreenSize(deviceUDID) {
    const conn = activeWebRTCConnections.get(deviceUDID);
    if (conn && conn.video) {
        if (conn.video.videoWidth > 0 && conn.video.videoHeight > 0) {
            return { width: conn.video.videoWidth, height: conn.video.videoHeight };
        }
    }
    return { width: 1080, height: 1920 }; // 默认值
}

// 3. 注入触摸事件
// pointerId 默认值：单指模式使用 SC_POINTER_ID_MOUSE (-1)，与 scrcpy 一致
function injectTouchEvent(deviceUDID, x, y, action = MotionAction.DOWN, pointerId = -1) {
    const pressure = 0xFFFF;
    const actionButton = 0;
    const buttons = (action === MotionAction.DOWN || 
                     action === MotionAction.POINTER_DOWN || 
                     action === MotionAction.MOVE) ? 1 : 0;
    
    const { width: screenWidth, height: screenHeight } = getDeviceScreenSize(deviceUDID);
    
    const data = new Uint8Array([
        action,
        ...toBigEndianBytes(pointerId, 8),
        ...toBigEndianBytes(x, 4),
        ...toBigEndianBytes(y, 4),
        ...toBigEndianBytes(screenWidth, 2),
        ...toBigEndianBytes(screenHeight, 2),
        ...toBigEndianBytes(pressure, 2),
        ...toBigEndianBytes(actionButton, 4),
        ...toBigEndianBytes(buttons, 4)
    ]);
    
    return sendControlMessage(deviceUDID, ControlMessageType.INJECT_TOUCH_EVENT, data);
}

// 4. 注入滚动事件
function injectScrollEvent(deviceUDID, x, y, hscroll, vscroll) {
    const buttons = 0;
    const { width: screenWidth, height: screenHeight } = getDeviceScreenSize(deviceUDID);
    
    // scrcpy: 滚动值范围 -16 到 16，归一化到 -1 到 1，转换为 int16_t 定点数
    const hscrollNorm = Math.max(-1, Math.min(1, hscroll / 16));
    const vscrollNorm = Math.max(-1, Math.min(1, vscroll / 16));
    const hscrollFixed = Math.round(hscrollNorm * 0x8000);
    const vscrollFixed = Math.round(vscrollNorm * 0x8000);
    const hscrollValue = Math.max(-0x8000, Math.min(0x7FFF, hscrollFixed));
    const vscrollValue = Math.max(-0x8000, Math.min(0x7FFF, vscrollFixed));
    const hscrollU16 = hscrollValue < 0 ? (0x10000 + hscrollValue) : hscrollValue;
    const vscrollU16 = vscrollValue < 0 ? (0x10000 + vscrollValue) : vscrollValue;
    
    // position 字段: x(4) + y(4) + screenWidth(2) + screenHeight(2) = 12字节
    const data = new Uint8Array([
        ...toBigEndianBytes(x, 4),
        ...toBigEndianBytes(y, 4),
        ...toBigEndianBytes(screenWidth, 2),
        ...toBigEndianBytes(screenHeight, 2),
        ...toBigEndianBytes(hscrollU16, 2),
        ...toBigEndianBytes(vscrollU16, 2),
        ...toBigEndianBytes(buttons, 4)
    ]);
    
    return sendControlMessage(deviceUDID, ControlMessageType.INJECT_SCROLL_EVENT, data);
}

// 5. 返回键或唤醒屏幕
function backOrScreenOn(deviceUDID, action = MotionAction.DOWN) {
    const data = new Uint8Array([action]);
    return sendControlMessage(deviceUDID, ControlMessageType.BACK_OR_SCREEN_ON, data);
}

// 6-7. 系统面板控制（无数据）
function expandNotificationPanel(deviceUDID) {
    return sendControlMessage(deviceUDID, ControlMessageType.EXPAND_NOTIFICATION_PANEL);
}

function expandSettingsPanel(deviceUDID) {
    return sendControlMessage(deviceUDID, ControlMessageType.EXPAND_SETTINGS_PANEL);
}

function collapsePanels(deviceUDID) {
    return sendControlMessage(deviceUDID, ControlMessageType.COLLAPSE_PANELS);
}

// 8. 获取剪贴板
function getClipboard(deviceUDID, copyKey = 0) {
    const data = new Uint8Array([copyKey]);
    return sendControlMessage(deviceUDID, ControlMessageType.GET_CLIPBOARD, data);
}

// 9. 设置剪贴板
function setClipboard(deviceUDID, text, paste = false) {
    const sequence = Date.now(); // 使用时间戳作为序列号
    const textBytes = stringToUtf8Bytes(text);
    // scrcpy使用4字节big-endian长度（parseSetClipboard -> parseString() -> parseString(4)）
    const lengthBytes = toBigEndianBytes(textBytes.length, 4);
    const data = new Uint8Array([
        ...toBigEndianBytes(sequence, 8),
        paste ? 1 : 0,
        ...lengthBytes,
        ...textBytes
    ]);
    return sendControlMessage(deviceUDID, ControlMessageType.SET_CLIPBOARD, data);
}

// 10. 设置屏幕电源
function setDisplayPower(deviceUDID, on = true, toastMessage = null) {
    const data = new Uint8Array([on ? 1 : 0]);
    return sendControlMessage(deviceUDID, ControlMessageType.SET_DISPLAY_POWER, data, toastMessage);
}

// 11. 旋转设备（无数据）
function rotateDevice(deviceUDID) {
    return sendControlMessage(deviceUDID, ControlMessageType.ROTATE_DEVICE, new Uint8Array(0), '已发送旋转指令');
}

// 12. 创建 UHID 设备
// 参数：
//   - id: 设备ID (2字节，0-65535)
//   - vendorId: 厂商ID (2字节，0-65535)
//   - productId: 产品ID (2字节，0-65535)
//   - name: 设备名称 (字符串，UTF-8)
//   - reportDesc: 报告描述符 (Uint8Array 或 Array)
function createUhid(deviceUDID, id, vendorId, productId, name, reportDesc) {
    const nameBytes = stringToUtf8Bytes(name);
    const reportDescBytes = reportDesc instanceof Uint8Array ? reportDesc : new Uint8Array(reportDesc);
    
    // 构建数据：id(2) + vendor_id(2) + product_id(2) + name长度(1) + name内容 + report_desc长度(1) + report_desc内容
    const data = new Uint8Array([
        ...toBigEndianBytes(id, 2),
        ...toBigEndianBytes(vendorId, 2),
        ...toBigEndianBytes(productId, 2),
        ...toBigEndianBytes(nameBytes.length, 1),
        ...nameBytes,
        ...toBigEndianBytes(reportDescBytes.length, 1),
        ...reportDescBytes
    ]);
    return sendControlMessage(deviceUDID, ControlMessageType.UHID_CREATE, data);
}

// 13. UHID 输入
// 参数：
//   - id: 设备ID (2字节，0-65535)
//   - data: 输入数据 (Uint8Array 或 Array)
function uhidInput(deviceUDID, id, data) {
    const inputData = data instanceof Uint8Array ? data : new Uint8Array(data);
    const dataBytes = new Uint8Array([
        ...toBigEndianBytes(id, 2),
        ...toBigEndianBytes(inputData.length, 2),
        ...inputData
    ]);
    return sendControlMessage(deviceUDID, ControlMessageType.UHID_INPUT, dataBytes);
}

// 14. 销毁 UHID 设备
// 参数：
//   - id: 设备ID (2字节，0-65535)
function destroyUhid(deviceUDID, id) {
    const data = toBigEndianBytes(id, 2);
    return sendControlMessage(deviceUDID, ControlMessageType.UHID_DESTROY, data);
}

// 15. 打开硬件键盘设置（无数据）
function openHardKeyboardSettings(deviceUDID) {
    return sendControlMessage(deviceUDID, ControlMessageType.OPEN_HARD_KEYBOARD_SETTINGS);
}

// 16. 启动应用
function startApp(deviceUDID, appName) {
    const appBytes = stringToUtf8Bytes(appName);
    // scrcpy使用1字节长度（parseStartApp -> parseString(1)，对应 write_string_tiny）
    const lengthBytes = toBigEndianBytes(appBytes.length, 1);
    const data = new Uint8Array([...lengthBytes, ...appBytes]);
    return sendControlMessage(deviceUDID, ControlMessageType.START_APP, data);
}

// 17. 重置视频（请求关键帧，无数据）
function resetVideo(deviceUDID) {
    return sendControlMessage(deviceUDID, ControlMessageType.RESET_VIDEO);
}

// 导出所有控制函数
window.ControlCommands = {
    injectKeycode,
    injectText,
    injectTouchEvent,
    injectScrollEvent,
    backOrScreenOn,
    expandNotificationPanel,
    expandSettingsPanel,
    collapsePanels,
    getClipboard,
    setClipboard,
    setDisplayPower,
    rotateDevice,
    createUhid,
    uhidInput,
    destroyUhid,
    openHardKeyboardSettings,
    startApp,
    resetVideo,
    getDeviceScreenSize,
    toBigEndianBytes,
    KeyCode,
    MotionAction,
    ControlMessageType,
    ClipboardCopyKey
};

