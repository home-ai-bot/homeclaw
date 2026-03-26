	FromID       string `json:"from_id"`
    From    string `json:"from"`  
	Name     string `json:"name"`
	Type    string `json:"type"`
	IP       string `json:"ip"`
	Token    string `json:"token"`
	URN      string `json:"urn,omitempty"`
	SpaceID   string `json:"space_id,omitempty"`
	SpaceName string `json:"space_name,omitempty"`
	Tags  string `json:"tags,omitempty"`
    Owners string
    Actions string   // "开灯:" {did:xxx,siid:xxx,aiid:xx,value:true},"关灯:" {did:xxx,siid:xxx,aiid:xx,value:false}: 解析后的参数