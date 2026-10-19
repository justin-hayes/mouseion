from google.protobuf.internal import containers as _containers
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from typing import ClassVar as _ClassVar, Iterable as _Iterable, Mapping as _Mapping, Optional as _Optional, Union as _Union

DESCRIPTOR: _descriptor.FileDescriptor

class GetCapabilitiesRequest(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class GetCapabilitiesResponse(_message.Message):
    __slots__ = ("languages",)
    LANGUAGES_FIELD_NUMBER: _ClassVar[int]
    languages: _containers.RepeatedCompositeFieldContainer[LanguageCapability]
    def __init__(self, languages: _Optional[_Iterable[_Union[LanguageCapability, _Mapping]]] = ...) -> None: ...

class LanguageCapability(_message.Message):
    __slots__ = ("language", "display_name", "model_version", "supported_features", "ready", "normalization_profile", "normalization_version")
    LANGUAGE_FIELD_NUMBER: _ClassVar[int]
    DISPLAY_NAME_FIELD_NUMBER: _ClassVar[int]
    MODEL_VERSION_FIELD_NUMBER: _ClassVar[int]
    SUPPORTED_FEATURES_FIELD_NUMBER: _ClassVar[int]
    READY_FIELD_NUMBER: _ClassVar[int]
    NORMALIZATION_PROFILE_FIELD_NUMBER: _ClassVar[int]
    NORMALIZATION_VERSION_FIELD_NUMBER: _ClassVar[int]
    language: str
    display_name: str
    model_version: str
    supported_features: _containers.RepeatedScalarFieldContainer[str]
    ready: bool
    normalization_profile: str
    normalization_version: str
    def __init__(self, language: _Optional[str] = ..., display_name: _Optional[str] = ..., model_version: _Optional[str] = ..., supported_features: _Optional[_Iterable[str]] = ..., ready: bool = ..., normalization_profile: _Optional[str] = ..., normalization_version: _Optional[str] = ...) -> None: ...

class AnalyzeRequest(_message.Message):
    __slots__ = ("language", "document_text", "source_document")
    LANGUAGE_FIELD_NUMBER: _ClassVar[int]
    DOCUMENT_TEXT_FIELD_NUMBER: _ClassVar[int]
    SOURCE_DOCUMENT_FIELD_NUMBER: _ClassVar[int]
    language: str
    document_text: str
    source_document: SourceDocument
    def __init__(self, language: _Optional[str] = ..., document_text: _Optional[str] = ..., source_document: _Optional[_Union[SourceDocument, _Mapping]] = ...) -> None: ...

class NormalizedCorpus(_message.Message):
    __slots__ = ("schema_version", "language", "sentences", "source_documents", "analysis", "normalization_profile")
    SCHEMA_VERSION_FIELD_NUMBER: _ClassVar[int]
    LANGUAGE_FIELD_NUMBER: _ClassVar[int]
    SENTENCES_FIELD_NUMBER: _ClassVar[int]
    SOURCE_DOCUMENTS_FIELD_NUMBER: _ClassVar[int]
    ANALYSIS_FIELD_NUMBER: _ClassVar[int]
    NORMALIZATION_PROFILE_FIELD_NUMBER: _ClassVar[int]
    schema_version: str
    language: str
    sentences: _containers.RepeatedCompositeFieldContainer[Sentence]
    source_documents: _containers.RepeatedCompositeFieldContainer[SourceDocument]
    analysis: AnalysisProvenance
    normalization_profile: NormalizationProfile
    def __init__(self, schema_version: _Optional[str] = ..., language: _Optional[str] = ..., sentences: _Optional[_Iterable[_Union[Sentence, _Mapping]]] = ..., source_documents: _Optional[_Iterable[_Union[SourceDocument, _Mapping]]] = ..., analysis: _Optional[_Union[AnalysisProvenance, _Mapping]] = ..., normalization_profile: _Optional[_Union[NormalizationProfile, _Mapping]] = ...) -> None: ...

class Sentence(_message.Message):
    __slots__ = ("text", "tokens", "location")
    TEXT_FIELD_NUMBER: _ClassVar[int]
    TOKENS_FIELD_NUMBER: _ClassVar[int]
    LOCATION_FIELD_NUMBER: _ClassVar[int]
    text: str
    tokens: _containers.RepeatedCompositeFieldContainer[Token]
    location: SourceLocation
    def __init__(self, text: _Optional[str] = ..., tokens: _Optional[_Iterable[_Union[Token, _Mapping]]] = ..., location: _Optional[_Union[SourceLocation, _Mapping]] = ...) -> None: ...

class Token(_message.Message):
    __slots__ = ("surface", "raw_lemma", "pos", "canonical_lemma", "morphology", "named_entity", "location", "dependency", "head")
    class MorphologyEntry(_message.Message):
        __slots__ = ("key", "value")
        KEY_FIELD_NUMBER: _ClassVar[int]
        VALUE_FIELD_NUMBER: _ClassVar[int]
        key: str
        value: str
        def __init__(self, key: _Optional[str] = ..., value: _Optional[str] = ...) -> None: ...
    SURFACE_FIELD_NUMBER: _ClassVar[int]
    RAW_LEMMA_FIELD_NUMBER: _ClassVar[int]
    POS_FIELD_NUMBER: _ClassVar[int]
    CANONICAL_LEMMA_FIELD_NUMBER: _ClassVar[int]
    MORPHOLOGY_FIELD_NUMBER: _ClassVar[int]
    NAMED_ENTITY_FIELD_NUMBER: _ClassVar[int]
    LOCATION_FIELD_NUMBER: _ClassVar[int]
    DEPENDENCY_FIELD_NUMBER: _ClassVar[int]
    HEAD_FIELD_NUMBER: _ClassVar[int]
    surface: str
    raw_lemma: str
    pos: str
    canonical_lemma: str
    morphology: _containers.ScalarMap[str, str]
    named_entity: str
    location: SourceLocation
    dependency: str
    head: int
    def __init__(self, surface: _Optional[str] = ..., raw_lemma: _Optional[str] = ..., pos: _Optional[str] = ..., canonical_lemma: _Optional[str] = ..., morphology: _Optional[_Mapping[str, str]] = ..., named_entity: _Optional[str] = ..., location: _Optional[_Union[SourceLocation, _Mapping]] = ..., dependency: _Optional[str] = ..., head: _Optional[int] = ...) -> None: ...

class SourceDocument(_message.Message):
    __slots__ = ("id", "source_identifier", "title")
    ID_FIELD_NUMBER: _ClassVar[int]
    SOURCE_IDENTIFIER_FIELD_NUMBER: _ClassVar[int]
    TITLE_FIELD_NUMBER: _ClassVar[int]
    id: str
    source_identifier: str
    title: str
    def __init__(self, id: _Optional[str] = ..., source_identifier: _Optional[str] = ..., title: _Optional[str] = ...) -> None: ...

class AnalysisProvenance(_message.Message):
    __slots__ = ("run_id", "analyzed_at", "analyzer_name", "analyzer_version")
    RUN_ID_FIELD_NUMBER: _ClassVar[int]
    ANALYZED_AT_FIELD_NUMBER: _ClassVar[int]
    ANALYZER_NAME_FIELD_NUMBER: _ClassVar[int]
    ANALYZER_VERSION_FIELD_NUMBER: _ClassVar[int]
    run_id: str
    analyzed_at: str
    analyzer_name: str
    analyzer_version: str
    def __init__(self, run_id: _Optional[str] = ..., analyzed_at: _Optional[str] = ..., analyzer_name: _Optional[str] = ..., analyzer_version: _Optional[str] = ...) -> None: ...

class NormalizationProfile(_message.Message):
    __slots__ = ("name", "version")
    NAME_FIELD_NUMBER: _ClassVar[int]
    VERSION_FIELD_NUMBER: _ClassVar[int]
    name: str
    version: str
    def __init__(self, name: _Optional[str] = ..., version: _Optional[str] = ...) -> None: ...

class SourceLocation(_message.Message):
    __slots__ = ("source_document_id", "chapter", "section", "start_offset", "end_offset")
    SOURCE_DOCUMENT_ID_FIELD_NUMBER: _ClassVar[int]
    CHAPTER_FIELD_NUMBER: _ClassVar[int]
    SECTION_FIELD_NUMBER: _ClassVar[int]
    START_OFFSET_FIELD_NUMBER: _ClassVar[int]
    END_OFFSET_FIELD_NUMBER: _ClassVar[int]
    source_document_id: str
    chapter: str
    section: str
    start_offset: int
    end_offset: int
    def __init__(self, source_document_id: _Optional[str] = ..., chapter: _Optional[str] = ..., section: _Optional[str] = ..., start_offset: _Optional[int] = ..., end_offset: _Optional[int] = ...) -> None: ...
