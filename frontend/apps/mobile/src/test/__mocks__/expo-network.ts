// Mock for expo-network: jsdom doesn't have the globalThis.expo bridge so
// the real module can't be imported. usePlaySession only calls
// getNetworkStateAsync() to label the request as wifi/cellular; in tests
// we always return wifi.
export enum NetworkStateType {
  UNKNOWN = "UNKNOWN",
  NONE = "NONE",
  CELLULAR = "CELLULAR",
  WIFI = "WIFI",
  BLUETOOTH = "BLUETOOTH",
  ETHERNET = "ETHERNET",
  VPN = "VPN",
  OTHER = "OTHER",
}

export async function getNetworkStateAsync() {
  return { type: NetworkStateType.WIFI, isConnected: true, isInternetReachable: true };
}
