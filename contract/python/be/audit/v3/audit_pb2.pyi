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

class EvidenceStatus(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    EVIDENCE_STATUS_UNSPECIFIED: _ClassVar[EvidenceStatus]
    EVIDENCE_STATUS_ACCEPTED: _ClassVar[EvidenceStatus]
    EVIDENCE_STATUS_DUPLICATE: _ClassVar[EvidenceStatus]
    EVIDENCE_STATUS_CONFLICT: _ClassVar[EvidenceStatus]
    EVIDENCE_STATUS_INVALID: _ClassVar[EvidenceStatus]
RECEIPT_STATUS_UNSPECIFIED: ReceiptStatus
RECEIPT_STATUS_RECEIVED: ReceiptStatus
RECEIPT_STATUS_CONFLICT: ReceiptStatus
RECEIPT_STATUS_INVALID: ReceiptStatus
EVIDENCE_STATUS_UNSPECIFIED: EvidenceStatus
EVIDENCE_STATUS_ACCEPTED: EvidenceStatus
EVIDENCE_STATUS_DUPLICATE: EvidenceStatus
EVIDENCE_STATUS_CONFLICT: EvidenceStatus
EVIDENCE_STATUS_INVALID: EvidenceStatus

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

class Evidence(_message.Message):
    __slots__ = ("record", "digest")
    RECORD_FIELD_NUMBER: _ClassVar[int]
    DIGEST_FIELD_NUMBER: _ClassVar[int]
    record: bytes
    digest: str
    def __init__(self, record: _Optional[bytes] = ..., digest: _Optional[str] = ...) -> None: ...

class AppendRequest(_message.Message):
    __slots__ = ("records",)
    RECORDS_FIELD_NUMBER: _ClassVar[int]
    records: _containers.RepeatedCompositeFieldContainer[Evidence]
    def __init__(self, records: _Optional[_Iterable[_Union[Evidence, _Mapping]]] = ...) -> None: ...

class EvidenceReceipt(_message.Message):
    __slots__ = ("id", "phase", "status", "epoch", "seq")
    ID_FIELD_NUMBER: _ClassVar[int]
    PHASE_FIELD_NUMBER: _ClassVar[int]
    STATUS_FIELD_NUMBER: _ClassVar[int]
    EPOCH_FIELD_NUMBER: _ClassVar[int]
    SEQ_FIELD_NUMBER: _ClassVar[int]
    id: str
    phase: str
    status: EvidenceStatus
    epoch: str
    seq: int
    def __init__(self, id: _Optional[str] = ..., phase: _Optional[str] = ..., status: _Optional[_Union[EvidenceStatus, str]] = ..., epoch: _Optional[str] = ..., seq: _Optional[int] = ...) -> None: ...

class AppendResponse(_message.Message):
    __slots__ = ("receipts",)
    RECEIPTS_FIELD_NUMBER: _ClassVar[int]
    receipts: _containers.RepeatedCompositeFieldContainer[EvidenceReceipt]
    def __init__(self, receipts: _Optional[_Iterable[_Union[EvidenceReceipt, _Mapping]]] = ...) -> None: ...

class ListRequest(_message.Message):
    __slots__ = ("after_seq", "recorder", "category", "limit")
    AFTER_SEQ_FIELD_NUMBER: _ClassVar[int]
    RECORDER_FIELD_NUMBER: _ClassVar[int]
    CATEGORY_FIELD_NUMBER: _ClassVar[int]
    LIMIT_FIELD_NUMBER: _ClassVar[int]
    after_seq: int
    recorder: str
    category: str
    limit: int
    def __init__(self, after_seq: _Optional[int] = ..., recorder: _Optional[str] = ..., category: _Optional[str] = ..., limit: _Optional[int] = ...) -> None: ...

class StoredEvidence(_message.Message):
    __slots__ = ("evidence", "epoch", "seq", "received_at")
    EVIDENCE_FIELD_NUMBER: _ClassVar[int]
    EPOCH_FIELD_NUMBER: _ClassVar[int]
    SEQ_FIELD_NUMBER: _ClassVar[int]
    RECEIVED_AT_FIELD_NUMBER: _ClassVar[int]
    evidence: Evidence
    epoch: str
    seq: int
    received_at: str
    def __init__(self, evidence: _Optional[_Union[Evidence, _Mapping]] = ..., epoch: _Optional[str] = ..., seq: _Optional[int] = ..., received_at: _Optional[str] = ...) -> None: ...

class ListResponse(_message.Message):
    __slots__ = ("records", "more")
    RECORDS_FIELD_NUMBER: _ClassVar[int]
    MORE_FIELD_NUMBER: _ClassVar[int]
    records: _containers.RepeatedCompositeFieldContainer[StoredEvidence]
    more: bool
    def __init__(self, records: _Optional[_Iterable[_Union[StoredEvidence, _Mapping]]] = ..., more: _Optional[bool] = ...) -> None: ...
