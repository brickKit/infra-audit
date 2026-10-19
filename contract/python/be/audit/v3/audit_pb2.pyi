from google.protobuf.internal import containers as _containers
from google.protobuf.internal import enum_type_wrapper as _enum_type_wrapper
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from collections.abc import Iterable as _Iterable, Mapping as _Mapping
from typing import ClassVar as _ClassVar, Optional as _Optional, Union as _Union

DESCRIPTOR: _descriptor.FileDescriptor

class ReceiptStatus(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    RECEIPT_STATUS_UNSPECIFIED: _ClassVar[ReceiptStatus]
    RECEIPT_STATUS_RECEIVED: _ClassVar[ReceiptStatus]
    RECEIPT_STATUS_CONFLICT: _ClassVar[ReceiptStatus]
    RECEIPT_STATUS_INVALID: _ClassVar[ReceiptStatus]
RECEIPT_STATUS_UNSPECIFIED: ReceiptStatus
RECEIPT_STATUS_RECEIVED: ReceiptStatus
RECEIPT_STATUS_CONFLICT: ReceiptStatus
RECEIPT_STATUS_INVALID: ReceiptStatus

class Item(_message.Message):
    __slots__ = ("id", "seq", "epoch", "digest", "record")
    ID_FIELD_NUMBER: _ClassVar[int]
    SEQ_FIELD_NUMBER: _ClassVar[int]
    EPOCH_FIELD_NUMBER: _ClassVar[int]
    DIGEST_FIELD_NUMBER: _ClassVar[int]
    RECORD_FIELD_NUMBER: _ClassVar[int]
    id: str
    seq: int
    epoch: str
    digest: str
    record: bytes
    def __init__(self, id: _Optional[str] = ..., seq: _Optional[int] = ..., epoch: _Optional[str] = ..., digest: _Optional[str] = ..., record: _Optional[bytes] = ...) -> None: ...

class SubmitRequest(_message.Message):
    __slots__ = ("items",)
    ITEMS_FIELD_NUMBER: _ClassVar[int]
    items: _containers.RepeatedCompositeFieldContainer[Item]
    def __init__(self, items: _Optional[_Iterable[_Union[Item, _Mapping]]] = ...) -> None: ...

class Receipt(_message.Message):
    __slots__ = ("id", "status", "epoch", "seq", "chain")
    ID_FIELD_NUMBER: _ClassVar[int]
    STATUS_FIELD_NUMBER: _ClassVar[int]
    EPOCH_FIELD_NUMBER: _ClassVar[int]
    SEQ_FIELD_NUMBER: _ClassVar[int]
    CHAIN_FIELD_NUMBER: _ClassVar[int]
    id: str
    status: ReceiptStatus
    epoch: str
    seq: int
    chain: str
    def __init__(self, id: _Optional[str] = ..., status: _Optional[_Union[ReceiptStatus, str]] = ..., epoch: _Optional[str] = ..., seq: _Optional[int] = ..., chain: _Optional[str] = ...) -> None: ...

class SubmitResponse(_message.Message):
    __slots__ = ("receipts",)
    RECEIPTS_FIELD_NUMBER: _ClassVar[int]
    receipts: _containers.RepeatedCompositeFieldContainer[Receipt]
    def __init__(self, receipts: _Optional[_Iterable[_Union[Receipt, _Mapping]]] = ...) -> None: ...

class WatermarkRequest(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class Ancestor(_message.Message):
    __slots__ = ("epoch", "fork")
    EPOCH_FIELD_NUMBER: _ClassVar[int]
    FORK_FIELD_NUMBER: _ClassVar[int]
    epoch: str
    fork: int
    def __init__(self, epoch: _Optional[str] = ..., fork: _Optional[int] = ...) -> None: ...

class WatermarkResponse(_message.Message):
    __slots__ = ("epoch", "seq", "ancestors", "backed_up_in_epoch")
    EPOCH_FIELD_NUMBER: _ClassVar[int]
    SEQ_FIELD_NUMBER: _ClassVar[int]
    ANCESTORS_FIELD_NUMBER: _ClassVar[int]
    BACKED_UP_IN_EPOCH_FIELD_NUMBER: _ClassVar[int]
    epoch: str
    seq: int
    ancestors: _containers.RepeatedCompositeFieldContainer[Ancestor]
    backed_up_in_epoch: bool
    def __init__(self, epoch: _Optional[str] = ..., seq: _Optional[int] = ..., ancestors: _Optional[_Iterable[_Union[Ancestor, _Mapping]]] = ..., backed_up_in_epoch: _Optional[bool] = ...) -> None: ...
