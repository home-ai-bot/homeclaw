1. 参考springboot的event机制,实现event Publish scribe机制
2. event类型有 device\room\prop\DeviceMsg\token\net 等变化
3. 所有的变化事件都发送到统一的事件中心,所有的订阅都从事件中心订阅