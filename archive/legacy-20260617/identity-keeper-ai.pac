function FindProxyForURL(url, host) {
  host = host.toLowerCase();

  var identityKeeper = "PROXY 127.0.0.1:18080";
  var existingProxy = "PROXY 127.0.0.1:7889; DIRECT";

  if (
    dnsDomainIs(host, "claude.ai") ||
    shExpMatch(host, "*.claude.ai") ||
    dnsDomainIs(host, "anthropic.com") ||
    shExpMatch(host, "*.anthropic.com") ||
    dnsDomainIs(host, "openai.com") ||
    shExpMatch(host, "*.openai.com") ||
    dnsDomainIs(host, "chatgpt.com") ||
    shExpMatch(host, "*.chatgpt.com") ||
    dnsDomainIs(host, "cursor.com") ||
    shExpMatch(host, "*.cursor.com") ||
    dnsDomainIs(host, "cursor.sh") ||
    shExpMatch(host, "*.cursor.sh") ||
    dnsDomainIs(host, "ipinfo.io") ||
    shExpMatch(host, "*.ipinfo.io") ||
    dnsDomainIs(host, "ping0.cc") ||
    shExpMatch(host, "*.ping0.cc") ||
    dnsDomainIs(host, "ip.sb") ||
    shExpMatch(host, "*.ip.sb") ||
    dnsDomainIs(host, "ifconfig.me") ||
    shExpMatch(host, "*.ifconfig.me")
  ) {
    return identityKeeper;
  }

  return existingProxy;
}
