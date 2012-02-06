import os
from operator import attrgetter
from zipfile import ZipFile

from fb2_metadata_collector import FB2MetadataCollector, InvalidFB2
from book import Book


class UnsupportedBookFileType(Exception):
    pass


class BookFileProcessorFactory(object):
    def __init__(self, path):
        self.path = path

    def get_book_file_processor(self):
        BookFileProcessorClass = self.get_book_file_processor_class()
        return BookFileProcessorClass(self.path)

    def get_book_file_processor_class(self):
        try:
            return {
                '.fb2': FB2BookFileProcessor,
                '.zip': ZippedFB2BookFileProcessor,
            }[self.get_file_extension()]
        except KeyError, e:
            raise UnsupportedBookFileType(str(e))

    def get_file_extension(self):
        name, ext = os.path.splitext(self.path)
        return ext


class BaseBookFileProcessor(object):
    def __init__(self, path):
        self.path = path

    def build_book(self, book_metadata_collector):
        return Book(
            path=self.path,
            author_first_name=book_metadata_collector.get_author_first_name(),
            author_middle_name=book_metadata_collector.get_author_middle_name(),
            author_last_name=book_metadata_collector.get_author_last_name(),
            title=book_metadata_collector.get_title(),
            genre=book_metadata_collector.get_genre(),
            date=book_metadata_collector.get_date(),
            language=book_metadata_collector.get_language()
        )

    def get_book(self):
        try:
            self.validate_book_file()
            book_metadata_collector = self.get_book_metadata_collector()
            return self.build_book(book_metadata_collector)
        except Exception, e:
            raise UnsupportedBookFileType()


    def validate_book_file(self):
        pass

    def get_book_metadata_collector(self):
        raise NotImplementedError


class FB2BookFileProcessor(BaseBookFileProcessor):
    def get_book_metadata_collector(self):
        return FB2MetadataCollector.create_instance_by_filename(
            self.path
        )


class ZippedFB2BookFileProcessor(BaseBookFileProcessor):
    def validate_book_file(self):
        if not self.is_zip_contains_single_fb2():
            raise UnsupportedBookFileType('.fb2.zip file has to contain single .fb2 file')

    def get_book_metadata_collector(self):
        return FB2MetadataCollector.create_instance_by_file(
            self.get_zipped_fb2_file()
        )

    def is_zip_contains_single_fb2(self):
        file_list = self.get_file_list()
        if len(file_list) != 1:
            return False
        filename = file_list[0]
        if not filename.endswith('.fb2'):
            return False
        return True

    def get_zipped_fb2_file(self):
        return self.get_zipfile().open(self.get_zipped_fb2_filename())

    def get_zipped_fb2_filename(self):
        return self.get_file_list()[0]

    def get_file_list(self):
        return map(attrgetter('filename'), self.get_zipfile().filelist)

    def get_zipfile(self):
        return ZipFile(self.path)
