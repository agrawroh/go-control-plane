// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.22.0
// 	protoc        v3.13.0
// source: envoy/admin/v4alpha/init_dump.proto

package envoy_admin_v4alpha

import (
	_ "github.com/cncf/udpa/go/udpa/annotations"
	proto "github.com/golang/protobuf/proto"
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	reflect "reflect"
	sync "sync"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

// This is a compile-time assertion that a sufficiently up-to-date version
// of the legacy proto package is being used.
const _ = proto.ProtoPackageIsVersion4

// Dumps of unready targets of envoy init managers. Envoy's admin fills this message with init managers,
// which provides the information of their unready targets.
// The :ref:`/init_dump <operations_admin_interface_init_dump>` will dump all unready targets information.
type UnreadyTargetsDumps struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// You can choose specific component to dump unready targets with mask query parameter.
	// See :ref:`/init_dump?mask={} <operations_admin_interface_init_dump_by_mask>` for more information.
	// The dumps of unready targets of all init managers.
	UnreadyTargetsDumps []*UnreadyTargetsDumps_UnreadyTargetsDump `protobuf:"bytes,1,rep,name=unready_targets_dumps,json=unreadyTargetsDumps,proto3" json:"unready_targets_dumps,omitempty"`
}

func (x *UnreadyTargetsDumps) Reset() {
	*x = UnreadyTargetsDumps{}
	if protoimpl.UnsafeEnabled {
		mi := &file_envoy_admin_v4alpha_init_dump_proto_msgTypes[0]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *UnreadyTargetsDumps) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*UnreadyTargetsDumps) ProtoMessage() {}

func (x *UnreadyTargetsDumps) ProtoReflect() protoreflect.Message {
	mi := &file_envoy_admin_v4alpha_init_dump_proto_msgTypes[0]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use UnreadyTargetsDumps.ProtoReflect.Descriptor instead.
func (*UnreadyTargetsDumps) Descriptor() ([]byte, []int) {
	return file_envoy_admin_v4alpha_init_dump_proto_rawDescGZIP(), []int{0}
}

func (x *UnreadyTargetsDumps) GetUnreadyTargetsDumps() []*UnreadyTargetsDumps_UnreadyTargetsDump {
	if x != nil {
		return x.UnreadyTargetsDumps
	}
	return nil
}

// Message of unready targets information of an init manager.
type UnreadyTargetsDumps_UnreadyTargetsDump struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// Name of the init manager. Example: "init_manager_xxx".
	Name string `protobuf:"bytes,1,opt,name=name,proto3" json:"name,omitempty"`
	// Names of unready targets of the init manager. Example: "target_xxx".
	TargetNames []string `protobuf:"bytes,2,rep,name=target_names,json=targetNames,proto3" json:"target_names,omitempty"`
}

func (x *UnreadyTargetsDumps_UnreadyTargetsDump) Reset() {
	*x = UnreadyTargetsDumps_UnreadyTargetsDump{}
	if protoimpl.UnsafeEnabled {
		mi := &file_envoy_admin_v4alpha_init_dump_proto_msgTypes[1]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *UnreadyTargetsDumps_UnreadyTargetsDump) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*UnreadyTargetsDumps_UnreadyTargetsDump) ProtoMessage() {}

func (x *UnreadyTargetsDumps_UnreadyTargetsDump) ProtoReflect() protoreflect.Message {
	mi := &file_envoy_admin_v4alpha_init_dump_proto_msgTypes[1]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use UnreadyTargetsDumps_UnreadyTargetsDump.ProtoReflect.Descriptor instead.
func (*UnreadyTargetsDumps_UnreadyTargetsDump) Descriptor() ([]byte, []int) {
	return file_envoy_admin_v4alpha_init_dump_proto_rawDescGZIP(), []int{0, 0}
}

func (x *UnreadyTargetsDumps_UnreadyTargetsDump) GetName() string {
	if x != nil {
		return x.Name
	}
	return ""
}

func (x *UnreadyTargetsDumps_UnreadyTargetsDump) GetTargetNames() []string {
	if x != nil {
		return x.TargetNames
	}
	return nil
}

var File_envoy_admin_v4alpha_init_dump_proto protoreflect.FileDescriptor

var file_envoy_admin_v4alpha_init_dump_proto_rawDesc = []byte{
	0x0a, 0x23, 0x65, 0x6e, 0x76, 0x6f, 0x79, 0x2f, 0x61, 0x64, 0x6d, 0x69, 0x6e, 0x2f, 0x76, 0x34,
	0x61, 0x6c, 0x70, 0x68, 0x61, 0x2f, 0x69, 0x6e, 0x69, 0x74, 0x5f, 0x64, 0x75, 0x6d, 0x70, 0x2e,
	0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12, 0x13, 0x65, 0x6e, 0x76, 0x6f, 0x79, 0x2e, 0x61, 0x64, 0x6d,
	0x69, 0x6e, 0x2e, 0x76, 0x34, 0x61, 0x6c, 0x70, 0x68, 0x61, 0x1a, 0x1d, 0x75, 0x64, 0x70, 0x61,
	0x2f, 0x61, 0x6e, 0x6e, 0x6f, 0x74, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x73, 0x2f, 0x73, 0x74, 0x61,
	0x74, 0x75, 0x73, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x1a, 0x21, 0x75, 0x64, 0x70, 0x61, 0x2f,
	0x61, 0x6e, 0x6e, 0x6f, 0x74, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x73, 0x2f, 0x76, 0x65, 0x72, 0x73,
	0x69, 0x6f, 0x6e, 0x69, 0x6e, 0x67, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x22, 0xbd, 0x02, 0x0a,
	0x13, 0x55, 0x6e, 0x72, 0x65, 0x61, 0x64, 0x79, 0x54, 0x61, 0x72, 0x67, 0x65, 0x74, 0x73, 0x44,
	0x75, 0x6d, 0x70, 0x73, 0x12, 0x6f, 0x0a, 0x15, 0x75, 0x6e, 0x72, 0x65, 0x61, 0x64, 0x79, 0x5f,
	0x74, 0x61, 0x72, 0x67, 0x65, 0x74, 0x73, 0x5f, 0x64, 0x75, 0x6d, 0x70, 0x73, 0x18, 0x01, 0x20,
	0x03, 0x28, 0x0b, 0x32, 0x3b, 0x2e, 0x65, 0x6e, 0x76, 0x6f, 0x79, 0x2e, 0x61, 0x64, 0x6d, 0x69,
	0x6e, 0x2e, 0x76, 0x34, 0x61, 0x6c, 0x70, 0x68, 0x61, 0x2e, 0x55, 0x6e, 0x72, 0x65, 0x61, 0x64,
	0x79, 0x54, 0x61, 0x72, 0x67, 0x65, 0x74, 0x73, 0x44, 0x75, 0x6d, 0x70, 0x73, 0x2e, 0x55, 0x6e,
	0x72, 0x65, 0x61, 0x64, 0x79, 0x54, 0x61, 0x72, 0x67, 0x65, 0x74, 0x73, 0x44, 0x75, 0x6d, 0x70,
	0x52, 0x13, 0x75, 0x6e, 0x72, 0x65, 0x61, 0x64, 0x79, 0x54, 0x61, 0x72, 0x67, 0x65, 0x74, 0x73,
	0x44, 0x75, 0x6d, 0x70, 0x73, 0x1a, 0x89, 0x01, 0x0a, 0x12, 0x55, 0x6e, 0x72, 0x65, 0x61, 0x64,
	0x79, 0x54, 0x61, 0x72, 0x67, 0x65, 0x74, 0x73, 0x44, 0x75, 0x6d, 0x70, 0x12, 0x12, 0x0a, 0x04,
	0x6e, 0x61, 0x6d, 0x65, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x04, 0x6e, 0x61, 0x6d, 0x65,
	0x12, 0x21, 0x0a, 0x0c, 0x74, 0x61, 0x72, 0x67, 0x65, 0x74, 0x5f, 0x6e, 0x61, 0x6d, 0x65, 0x73,
	0x18, 0x02, 0x20, 0x03, 0x28, 0x09, 0x52, 0x0b, 0x74, 0x61, 0x72, 0x67, 0x65, 0x74, 0x4e, 0x61,
	0x6d, 0x65, 0x73, 0x3a, 0x3c, 0x9a, 0xc5, 0x88, 0x1e, 0x37, 0x0a, 0x35, 0x65, 0x6e, 0x76, 0x6f,
	0x79, 0x2e, 0x61, 0x64, 0x6d, 0x69, 0x6e, 0x2e, 0x76, 0x33, 0x2e, 0x55, 0x6e, 0x72, 0x65, 0x61,
	0x64, 0x79, 0x54, 0x61, 0x72, 0x67, 0x65, 0x74, 0x73, 0x44, 0x75, 0x6d, 0x70, 0x73, 0x2e, 0x55,
	0x6e, 0x72, 0x65, 0x61, 0x64, 0x79, 0x54, 0x61, 0x72, 0x67, 0x65, 0x74, 0x73, 0x44, 0x75, 0x6d,
	0x70, 0x3a, 0x29, 0x9a, 0xc5, 0x88, 0x1e, 0x24, 0x0a, 0x22, 0x65, 0x6e, 0x76, 0x6f, 0x79, 0x2e,
	0x61, 0x64, 0x6d, 0x69, 0x6e, 0x2e, 0x76, 0x33, 0x2e, 0x55, 0x6e, 0x72, 0x65, 0x61, 0x64, 0x79,
	0x54, 0x61, 0x72, 0x67, 0x65, 0x74, 0x73, 0x44, 0x75, 0x6d, 0x70, 0x73, 0x42, 0x3c, 0x0a, 0x21,
	0x69, 0x6f, 0x2e, 0x65, 0x6e, 0x76, 0x6f, 0x79, 0x70, 0x72, 0x6f, 0x78, 0x79, 0x2e, 0x65, 0x6e,
	0x76, 0x6f, 0x79, 0x2e, 0x61, 0x64, 0x6d, 0x69, 0x6e, 0x2e, 0x76, 0x34, 0x61, 0x6c, 0x70, 0x68,
	0x61, 0x42, 0x0d, 0x49, 0x6e, 0x69, 0x74, 0x44, 0x75, 0x6d, 0x70, 0x50, 0x72, 0x6f, 0x74, 0x6f,
	0x50, 0x01, 0xba, 0x80, 0xc8, 0xd1, 0x06, 0x02, 0x10, 0x03, 0x62, 0x06, 0x70, 0x72, 0x6f, 0x74,
	0x6f, 0x33,
}

var (
	file_envoy_admin_v4alpha_init_dump_proto_rawDescOnce sync.Once
	file_envoy_admin_v4alpha_init_dump_proto_rawDescData = file_envoy_admin_v4alpha_init_dump_proto_rawDesc
)

func file_envoy_admin_v4alpha_init_dump_proto_rawDescGZIP() []byte {
	file_envoy_admin_v4alpha_init_dump_proto_rawDescOnce.Do(func() {
		file_envoy_admin_v4alpha_init_dump_proto_rawDescData = protoimpl.X.CompressGZIP(file_envoy_admin_v4alpha_init_dump_proto_rawDescData)
	})
	return file_envoy_admin_v4alpha_init_dump_proto_rawDescData
}

var file_envoy_admin_v4alpha_init_dump_proto_msgTypes = make([]protoimpl.MessageInfo, 2)
var file_envoy_admin_v4alpha_init_dump_proto_goTypes = []interface{}{
	(*UnreadyTargetsDumps)(nil),                    // 0: envoy.admin.v4alpha.UnreadyTargetsDumps
	(*UnreadyTargetsDumps_UnreadyTargetsDump)(nil), // 1: envoy.admin.v4alpha.UnreadyTargetsDumps.UnreadyTargetsDump
}
var file_envoy_admin_v4alpha_init_dump_proto_depIdxs = []int32{
	1, // 0: envoy.admin.v4alpha.UnreadyTargetsDumps.unready_targets_dumps:type_name -> envoy.admin.v4alpha.UnreadyTargetsDumps.UnreadyTargetsDump
	1, // [1:1] is the sub-list for method output_type
	1, // [1:1] is the sub-list for method input_type
	1, // [1:1] is the sub-list for extension type_name
	1, // [1:1] is the sub-list for extension extendee
	0, // [0:1] is the sub-list for field type_name
}

func init() { file_envoy_admin_v4alpha_init_dump_proto_init() }
func file_envoy_admin_v4alpha_init_dump_proto_init() {
	if File_envoy_admin_v4alpha_init_dump_proto != nil {
		return
	}
	if !protoimpl.UnsafeEnabled {
		file_envoy_admin_v4alpha_init_dump_proto_msgTypes[0].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*UnreadyTargetsDumps); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_envoy_admin_v4alpha_init_dump_proto_msgTypes[1].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*UnreadyTargetsDumps_UnreadyTargetsDump); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: file_envoy_admin_v4alpha_init_dump_proto_rawDesc,
			NumEnums:      0,
			NumMessages:   2,
			NumExtensions: 0,
			NumServices:   0,
		},
		GoTypes:           file_envoy_admin_v4alpha_init_dump_proto_goTypes,
		DependencyIndexes: file_envoy_admin_v4alpha_init_dump_proto_depIdxs,
		MessageInfos:      file_envoy_admin_v4alpha_init_dump_proto_msgTypes,
	}.Build()
	File_envoy_admin_v4alpha_init_dump_proto = out.File
	file_envoy_admin_v4alpha_init_dump_proto_rawDesc = nil
	file_envoy_admin_v4alpha_init_dump_proto_goTypes = nil
	file_envoy_admin_v4alpha_init_dump_proto_depIdxs = nil
}
