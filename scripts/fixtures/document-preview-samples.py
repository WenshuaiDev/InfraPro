"""Synthetic office/text fixtures for the ordinary attachment reader."""
from pathlib import Path
from io import BytesIO
import sys
import zipfile
import xml.etree.ElementTree as ET
from docx import Document
from docx.shared import Inches
from openpyxl import Workbook
from reportlab.pdfgen import canvas
from reportlab.lib.pagesizes import A4
from PIL import Image, ImageDraw

out = Path(sys.argv[1])
out.mkdir(parents=True, exist_ok=True)
picture = Image.new('RGB', (480, 160), '#f1f5f9')
draw = ImageDraw.Draw(picture)
draw.line([(20, 120), (130, 35), (240, 120), (350, 35), (460, 120)], fill='#2563eb', width=8)
draw.line([(20, 120), (460, 120)], fill='#334155', width=6)
buf = BytesIO()
picture.save(buf, format='PNG')
buf.seek(0)

doc = Document()
doc.sections[0].header.paragraphs[0].text = 'InfraPro · 合成预览资料'
doc.add_heading('青澜大桥巡检记录', 0)
doc.add_paragraph('第一部分：本资料用于在线预览自验。保留中文、段落、列表和表格。')
table = doc.add_table(rows=1, cols=3)
table.style = 'Table Grid'
for cell, value in zip(table.rows[0].cells, ['测点', '数值', '结论']): cell.text = value
for row in [['P-01', '12.5', '正常'], ['P-02', '14.8', '复核']]:
    for cell, value in zip(table.add_row().cells, row): cell.text = value
doc.add_page_break()
doc.add_heading('第二部分：图片与建议', 1)
doc.add_picture(buf, width=Inches(5))
doc.add_paragraph('检索关键词：伸缩缝。建议检查伸缩缝连接。')
doc.add_paragraph('保持记录完整', style='List Bullet')
doc.add_page_break()
doc.add_heading('第三部分：历史说明', 1)
doc.add_paragraph('预览修订 A。仅合成验收使用。')
doc.sections[0].footer.paragraphs[0].text = '合成资料，不含真实业务数据'
doc.save(out / 'preview.docx')

# External relationships and an HTML altChunk must never become console code or network access.
with zipfile.ZipFile(out / 'preview.docx') as source:
    hostile = {name: source.read(name) for name in source.namelist()}
w = 'http://schemas.openxmlformats.org/wordprocessingml/2006/main'
r = 'http://schemas.openxmlformats.org/officeDocument/2006/relationships'
relations = 'http://schemas.openxmlformats.org/package/2006/relationships'
xml = ET.fromstring(hostile['word/document.xml'])
body = xml.find(f'{{{w}}}body')
paragraph = ET.Element(f'{{{w}}}p')
link = ET.SubElement(paragraph, f'{{{w}}}hyperlink', {f'{{{r}}}id': 'previewExternal'})
ET.SubElement(ET.SubElement(link, f'{{{w}}}r'), f'{{{w}}}t').text = '外部链接测试'
body.insert(0, paragraph)
body.insert(1, ET.Element(f'{{{w}}}altChunk', {f'{{{r}}}id': 'previewChunk'}))
hostile['word/document.xml'] = ET.tostring(xml, encoding='utf-8', xml_declaration=True)
rels = ET.fromstring(hostile['word/_rels/document.xml.rels'])
ET.SubElement(rels, f'{{{relations}}}Relationship', {'Id': 'previewExternal', 'Type': r + '/hyperlink', 'Target': 'https://preview.invalid/should-never-load', 'TargetMode': 'External'})
ET.SubElement(rels, f'{{{relations}}}Relationship', {'Id': 'previewChunk', 'Type': r + '/aFChunk', 'Target': 'untrusted.html'})
hostile['word/_rels/document.xml.rels'] = ET.tostring(rels, encoding='utf-8', xml_declaration=True)
hostile['word/untrusted.html'] = b'<html><body><p>UNSAFE_ALTCHUNK</p><script>parent.document.body.dataset.pwned="yes"</script><img src="https://preview.invalid/should-never-load"></body></html>'
with zipfile.ZipFile(out / 'preview-untrusted.docx', 'w', zipfile.ZIP_DEFLATED) as target:
    for name, data in hostile.items(): target.writestr(name, data)

book = Workbook()
sheet = book.active
sheet.title = '观测数据'
sheet.append(['测点', '数值', '备注', '日期'])
for i in range(1, 125): sheet.append([f'P-{i:03d}', i / 10, '伸缩缝, 复核' if i == 90 else '正常', '2026-11-15'])
sheet['B2'] = '=SUM(B3:B4)'
sheet['C45'] = '跨页合并内容'
sheet.merge_cells('C45:C55')
summary = book.create_sheet('汇总说明')
summary['A1'] = '青澜大桥汇总'
summary.merge_cells('A1:C1')
summary['A3'] = '<script>alert("not executable")</script>'
summary['B3'] = '第二工作表'
book.save(out / 'preview.xlsx')
with zipfile.ZipFile(out / 'preview.xlsx') as source:
    members = {name: source.read(name) for name in source.namelist()}
members['xl/worksheets/sheet1.xml'] = members['xl/worksheets/sheet1.xml'].replace(b'<f>SUM(B3:B4)</f><v></v>', b'<f>SUM(B3:B4)</f><v>42</v>')
with zipfile.ZipFile(out / 'preview.xlsx', 'w', zipfile.ZIP_DEFLATED) as target:
    for name, data in members.items(): target.writestr(name, data)

pdf = canvas.Canvas(str(out / 'preview.pdf'), pagesize=A4)
for page in range(1, 4):
    pdf.setFont('Helvetica-Bold', 22)
    pdf.drawString(55, 770, f'InfraPro document preview - Page {page}')
    pdf.setFont('Helvetica', 14)
    pdf.drawString(55, 720, f'Bridge inspection {page}: expansion joint')
    pdf.rect(55, 470, 485, 180)
    pdf.drawString(70, 600, 'Synthetic drawing, text selection and search')
    pdf.showPage()
pdf.save()

text = '\n'.join(f'第 {i} 行：青澜大桥巡检记录' + (' 末尾定位关键词' if i == 701 else '') for i in range(1, 902))
(out / 'preview.txt').write_text(text, encoding='utf-8')
(out / 'preview-gbk.txt').write_bytes('中文编码验证\n桥梁与测点\n'.encode('gb18030'))
(out / 'preview-utf16.txt').write_bytes('UTF16 中文预览\r\n第二行'.encode('utf-16'))
(out / 'preview.csv').write_text('测点,备注,值\r\nP-01,"中文,逗号",00123\r\nP-02,"第一行\n第二行",=1+1\r\nP-03,"双引号""测试",12.50\r\n', encoding='utf-8-sig')
(out / 'broken.pdf').write_bytes(b'%PDF-1.7\nBroken structure\n%%EOF')
print('Created synthetic document fixtures.')
