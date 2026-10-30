package service

import (
	"fmt"
	"net"
	"strings"
)

// normalizeHost 校验并规范化对外主机名（推流主机、播放域名）：只接受合法域名或 IPv4 地址。
//
// 返回去掉首尾空白与末尾点、转为小写后的主机名。label 用于错误提示（如「推流主机」）。
// 不接受 IPv6：推流/播放地址直接拼接为 scheme://host:port，IPv6 需加方括号，暂不支持。
func normalizeHost(label, host string) (string, error) {
	h := strings.TrimSpace(host)
	if h == "" {
		return "", fmt.Errorf("%s不能为空", label)
	}
	if strings.Contains(h, "://") || strings.ContainsAny(h, "/?#@") {
		return "", fmt.Errorf("%s只需填写域名或 IP，不要带协议、路径或参数：%s", label, h)
	}
	// 中文输入法下常把「.」打成「，」「。」或英文逗号，单独提示
	if strings.ContainsAny(h, ",，。") {
		return "", fmt.Errorf("%s中含有逗号或句号，域名各段之间应使用英文点「.」：%s", label, h)
	}
	if ip := net.ParseIP(h); ip != nil {
		if ip.To4() == nil {
			return "", fmt.Errorf("%s暂不支持 IPv6 地址，请填写域名或 IPv4 地址：%s", label, h)
		}
		return ip.String(), nil
	}
	if strings.Contains(h, ":") {
		return "", fmt.Errorf("%s不要带端口（端口单独配置）：%s", label, h)
	}
	h = strings.ToLower(strings.TrimSuffix(h, "."))
	if len(h) > 253 {
		return "", fmt.Errorf("%s过长（最多 253 个字符）", label)
	}
	labels := strings.Split(h, ".")
	for _, l := range labels {
		if l == "" {
			return "", fmt.Errorf("%s格式不正确（出现连续的点或以点开头）：%s", label, host)
		}
		if len(l) > 63 {
			return "", fmt.Errorf("%s中每段最多 63 个字符：%s", label, host)
		}
		for _, r := range l {
			if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-') {
				return "", fmt.Errorf("%s只能包含英文字母、数字、中划线和点（不支持下划线、空格等字符）：%s", label, host)
			}
		}
		if l[0] == '-' || l[len(l)-1] == '-' {
			return "", fmt.Errorf("%s中每段不能以中划线开头或结尾：%s", label, host)
		}
	}
	// 形如 1.2.3 或 999.1.1.1 的纯数字点分串不是合法 IPv4，也不当作域名
	if allDigitsAndDots(h) {
		return "", fmt.Errorf("%s不是合法的 IPv4 地址：%s", label, host)
	}
	return h, nil
}

func allDigitsAndDots(s string) bool {
	for _, r := range s {
		if !(r >= '0' && r <= '9' || r == '.') {
			return false
		}
	}
	return true
}
